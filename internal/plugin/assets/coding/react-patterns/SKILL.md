---
name: react-patterns
description: Use when writing or changing React (Vite/TanStack) frontend code — component/state structure, list/table rendering gated on a total count, controlled forms, axios interceptor + timeout that exempts blob/arraybuffer downloads, ICU single-brace i18n, and env-driven feature gating.
allowed-tools: Read Grep Glob Bash
---

# React patterns

## Component / state

Keep local UI state (open/closed tabs, refs, in-flight form state) in `useState`/
`useRef` on the component; keep server state in a query hook, not in `useState`
fed by a `useEffect`. A form component composing several concerns typically has
three state groups side by side: local UI state, a form instance, and one or more
server queries feeding the form's options:

```jsx
const [activeTab, setActiveTab] = useState('details')
const [optionsByGroup, setOptionsByGroup] = useState({})

const form = useForm({
  resolver: zodResolver(formSchema),
  mode: 'onSubmit',
  defaultValues: { name: editing?.name || '', scope: editing?.scope || 'personal' },
})

const { data: groups = [] } = useQuery({
  queryKey: [QUERY_KEY.GET_GROUPS],
  queryFn: async () => {
    const res = await getGroups()
    return Array.isArray(res.data) ? res.data : []
  },
})
```

Do not duplicate query data into `useState` "to make it easier to mutate" — that
creates a second source of truth that goes stale the moment the query refetches.
Derive with `useMemo` instead of mirroring into state:

```jsx
const groupTabs = useMemo(
  () => groups.filter((g) => g.type !== 'SYSTEM'),
  [groups],
)
```

## List / table rendering gated on a total count

A paginated table component that reads row data from a `data` array and *also*
takes a separate `total` prop must render its body from `total`, not from
`data.length`. A common gate: `!isLoading && total > 0` renders rows,
`!isLoading && total === 0` renders the empty state — anything that leaves
`total` `undefined` (e.g. omitting the prop for a "just render what I have"
usage) makes BOTH branches false, so the table silently shows a header with an
empty body: no console error, no loading state stuck, no visible failure.

```jsx
{!isLoading && total > 0 ? (
  rows.map((row) => <TableRow key={row.id}>{/* ... */}</TableRow>)
) : null}
{!isLoading && total === 0 ? (
  <TableRow><TableCell colSpan={columnCount}>No data.</TableCell></TableRow>
) : null}
```

**Rule:** always pass `total` alongside `data`. For a client-side, non-paginated
list, pass `total={data.length}`. For a server-paginated list, pass the value the
server returned as the record count, not `data.length` (a page of 20 rows out of
2000 must still report 2000).

## Controlled form

Forms are built on a form-library instance (`react-hook-form` + a schema
resolver), never on ad hoc `useState` per field — the schema is the single
source of truth for both validation and default values, and error messages stay
next to the rule that produced them:

```jsx
const formSchema = z.object({
  name: z.string().refine((v) => v.trim().length > 0, {
    get message() {
      return i18n.t('form:label.nameRequired', { defaultValue: 'Name is required' })
    },
  }),
  scope: z.string().min(1).default('personal'),
})

const form = useForm({
  resolver: zodResolver(formSchema),
  mode: 'onSubmit',
  defaultValues: { name: editing?.name || '', scope: editing?.scope || 'personal' },
})
```

Field components read from `FormField`/`FormControl`/`FormMessage` wrappers built
around the form library's `Controller`, so every field gets id wiring
(`aria-describedby`, `aria-invalid`) and error rendering for free instead of each
field re-deriving it. `FormMessage` renders the field's `error.message` when
present, falling back to its `children` otherwise — a form-level submit handler
should never need to hand-roll per-field error display.

## Axios interceptor + timeout (exempt blob export)

A frontend that talks to several backends typically keeps one `axios.create()`
instance per backend, each carrying its own response interceptor pair — a
success interceptor that unwraps the backend's envelope, and an error
interceptor that classifies failures (auth-expired vs. permission-denied vs.
validation vs. transport) because different backends can disagree on which HTTP
status code means "your session is dead" versus "you lack permission for this
one action." Get that inversion wrong and either a permission error force-logs-out
a valid session, or an expired session never signs the user out.

If a shared instance also carries a client-side request `timeout` (anti-hang
protection against a dead backend), that timeout must be exempted for any
request whose `responseType` is `'blob'` or `'arraybuffer'` — a large, working
file export or report download is expected to legitimately run past a short
timeout ceiling, and the goal of the timeout is catching a request that never
resolves, not one that is merely slow. Applying the ceiling uniformly turns a
working export into a false failure:

```js
instance.interceptors.request.use((config) => {
  if (config.responseType === 'blob' || config.responseType === 'arraybuffer') {
    config.timeout = 0
  }
  return config
})
```

## i18n: ICU single-brace interpolation

When the i18n setup registers an ICU MessageFormat plugin, interpolation syntax
is **single-brace** (`{name}`), not the plain-i18next default of double-brace
(`{{name}}`). Using `{{name}}` under ICU does not error and does not warn — it
renders the literal characters `{{name}}` to the user, because ICU parses it as
plain text with no matching placeholder:

```js
// ICU-registered i18n instance:
t('routing:delete.confirm', {
  name: rule.name,
  defaultValue: 'Are you sure you want to delete {name}?', // correct: single brace
})
```

Before assuming an interpolated string renders correctly, check which
interpolation library the i18n instance is initialized with — a component
rendering without a console error is not proof the placeholder resolved; assert
on the rendered, interpolated text, not just that the element exists.

## Env-driven feature gate

A tenant- or org-scoped feature flag driven from build-time env config reads as:
an env var holds a comma-separated allowlist of ids, parsed once, and a helper
checks the current user's matching attribute against it. Absence of the env var
is a deliberate default (usually "enabled for everyone" rather than "disabled
for everyone"), so the empty-string case has to be handled explicitly rather
than falling through to an empty allowlist that denies everyone:

```js
export const isFeatureEnabledForOrg = (orgId) => {
  const raw = import.meta.env.APP_FEATURE_TENANT_IDS
  if (!raw) return true // unset = enabled everywhere, not "allowlist is empty"
  return raw
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
    .includes(orgId)
}
```

A gate keyed on a build-time env var only takes effect after a rebuild/redeploy
of that environment — it cannot be toggled at runtime, so treat it as a deploy
lever, not a live switch. Any UI surface guarding on this same flag (menu entry,
route, button) has to read it consistently, or a user can reach a route the menu
hid.

## Cross-refs

- Pair this skill with Vercel's community skill for React composition and
  performance idioms: `npx skills add vercel-labs/agent-skills --skill
  react-best-practices`.
- Backend counterparts: `nestjs-patterns` and `express-service-patterns` cover
  the API side of the contracts a React app consumes (response envelopes, error
  shapes, guards).
