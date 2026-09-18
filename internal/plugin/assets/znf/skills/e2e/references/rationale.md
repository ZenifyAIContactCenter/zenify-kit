# E2E — why each rule exists, and what the harness already does

## Why deep, not shallow

The UI checks the *user experience*; the API re-fetch checks the *domain truth*. A journey that
only asserts UI state is shallow, and a live run will out it.

## Why preconditions are seeded through the API

Clicking through ten screens to build data per test is the single most common AI-authored-test
anti-pattern. API seeding is fast, stable, and not flaky. Only the flow under test belongs in the
browser.

A unique per-run marker (a `runId`/UUID in the subject or name) is what lets parallel runs share
one staging environment without colliding — the precondition for turning on `fullyParallel` as the
suite grows.

## Why the doctrine rules matter

- **No `if`/`try` to hide failures.** `if (await x.isVisible())` around an action makes the test
  skip itself when the UI changes, producing a false green. This is the #1 AI-authored-test
  mistake.
- **Web-first assertions, always awaited.** `expect(await locator.isVisible()).toBe(true)` has no
  auto-retry, so it is flaky by construction.
- **Never assert implementation** — CSS classes, internal state, DOM order — because those change
  without the user-visible behaviour changing.
- **Eventually-consistent backends** (search index, replica lag) need `expect.poll`, never
  `waitForTimeout`, which either flakes or wastes time.
- **Thin helpers** keep assertions where the reader can see them. At this scale, fixtures beat the
  Page Object Model.

## Why the locator ladder is fixed

User-facing locators survive DOM and CSS changes; structural selectors do not. `data-testid` is
the last resort, valid only in three cases:

- the element is inside an **iframe / shadow DOM**, where `getByRole` cannot reach it (the TinyMCE
  editor is the standing example);
- there is **no stable accessible name** to anchor on;
- the **label is i18n-generated** and the test must be language-independent — `getByRole({name})`
  breaks when the app switches language, e.g. a submit button from `t('common:action.create')`. In
  an i18n-100% app this is a valid case.

**Adding a testid to production code:** on a shared component, never hardcode a feature-specific
testid — pass it via a prop (e.g. `dataTestId`) so only the specific call site renders it and the
testid does not leak onto the other usages. Remove dead testids: one added then abandoned looks
like a real anchor and is not.

## What the harness provides (don't rebuild it)

- **Auth**: logs in once via `auth.setup.ts` (setup project), saves `storageState`; every test
  starts authenticated — no per-test login. `apiClient` reads the raw token from storageState.
- **Cleanup runs even on failure**: `cleanupTracker` drains in fixture teardown (after `use()`), so
  a failed run leaks no data on shared staging. A failing undo only warns; it does not break other
  tests.
- **Trace**: `trace: 'retain-on-failure'` — every failed test has a trace; open it with
  `npx playwright show-trace`. (Not `on-first-retry`, because the suite runs `retries=0`.)
- **Isolation**: each test gets a fresh browser context. `fullyParallel:false` is the safe default
  for shared staging; turn on parallel once every journey is marker-isolated.

## Upstream documentation (authoritative)

- Best Practices — playwright.dev/docs/best-practices (locator priority, web-first, isolation, trace)
- Locators & test-id — playwright.dev/docs/locators (`getByRole → … → getByTestId`; testid last)
- API testing (seed / assert / cleanup via API) — playwright.dev/docs/api-testing
- Auth `storageState` — playwright.dev/docs/auth · Fixtures — playwright.dev/docs/test-fixtures
