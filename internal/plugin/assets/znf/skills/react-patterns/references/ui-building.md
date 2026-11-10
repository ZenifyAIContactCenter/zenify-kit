<!-- Read when: creating or restyling a React/Vue/Svelte component, before writing markup. -->

# Building UI that matches the repo

Hardcoded colors and re-built primitives are what make generated UI look foreign.
Ground in the repo first, then write.

## 1. Find the primitives before writing a component

1. If `components.json` exists, read `aliases.ui` for the primitive directory.
   Otherwise use `src/components/ui`.
2. `ls` that directory. Button, input, dialog, table, badge and select usually
   exist already; compose them instead of writing a new one.
3. Only add a new component when nothing there fits, and put it next to its peers.

## 2. Colors and sizes come from the theme

- Use theme classes (`text-primary`, `bg-background`, `border-border`) or CSS
  variables (`var(--primary)`).
- Do not write raw hex (`#ff0000`), Tailwind palette classes (`bg-red-500`,
  `text-slate-900/50`) or arbitrary sizes (`w-[437px]`, `mt-[1.5rem]`).
  Percent and fractional arbitrary values (`w-[50%]`, `grid-cols-[1fr_auto]`) are fine.
- A one-off value with no theme token: use the nearest token, or add the token
  to the theme. Do not inline the literal.

## 3. Strings go through the repo's i18n

Look for the existing hook or helper (`useTranslation`, `t(...)`, `$t(...)`) and
add keys to the locale files. No literal user-facing text in markup.

## 4. Check before you say done

Run the `ui-verify style` subcommand of the kit CLI (the one that provides `ui-verify check`):

```
<kit-cli> ui-verify style --repo <repo> --base <base ref>
```

It lints only the lines this branch added in `.tsx`, `.jsx`, `.vue` and `.svelte`
files, including brand-new untracked files, against the rules in
`uiVerify.style` of `.claude/worktree.json` (`hex`, `palette`, `arbitrary`).
Old violations are not counted. Exit 1 lists `file:line rule match`; fix them.
The same lint runs inside `ui-verify check`, and no other marker waives it.

## 5. Waiver for a real exception

Put `znf:ui-style-ok: <reason>` in a comment on the offending line or on the line
right above it. The reason is required. It waives that line and the next added
line only. Use it for a value that genuinely cannot be a token (a third-party
brand color), never to silence the check.
