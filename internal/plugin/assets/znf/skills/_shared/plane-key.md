# The Plane item line and the `zen-<a>-` slug — shared reference

Read when: a request is about a named Plane work item, before writing the spec header or
Intent, and before `wt new`.

## The `**Việc Plane:**` line

Format: `**Việc Plane:** ZEN-<a>[, ZEN-<b>…]`. The first key is the main item.

- Write it only when the request itself hands over that item (the user says "do ZEN-305" or
  "fix ZEN-250", or pastes a work item link).
- Never copy a key from a `**Liên quan:**` line, from prose, or from a description.
- `/cook`: the line sits in the spec header; the plan copies it.
- `/fix` and `/hotfix`: write the line as the first line of the ship-pack `## Intent`, before
  `wt new`. `/fix` Step 2b passes it verbatim in its handoff package
  (`fix/references/prd-routing.md`), and `/hotfix` copies it into its own `## Intent`.

## Slug

With the line present, the `wt new` slug starts with `zen-<a>-` (for example
`zen-305-session-timeout`). That prefix is also what fires the Plane In Progress hook
(`zenSlugRe`). Without the line, the slug is chosen as before.

## No key, no warning

An item that is not on Plane keeps the branch and the title as before. Do not warn, do not
invent a key.
