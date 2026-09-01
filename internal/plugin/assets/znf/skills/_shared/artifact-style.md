# Artifact style — shared reference

The rules every human-read artifact a skill produces must follow: specs, plans, design docs,
review write-ups. A badly-formatted artifact defeats its own purpose, so these are not optional
polish — they are part of "done". Skills such as `znf:brainstorming`, `znf:writing-plans`,
`znf:cook`, and `znf:writing-skills` cite this file instead of restating the rules.

These rules are project-agnostic. A project's own prose language and any domain term table live
in the project's convention, not here.

## Header metadata

Write each field as `**Label:** value` — the label in bold, the value immediately after on the
**same line** — one field per line, with a blank line between fields:

```
**Milestone:** M2 sub-project A

**Date:** 2026-09-02

**Author:** <name>

**Status:** Draft — awaiting approval
```

Three things this is NOT, each an actual mistake that has been caught:

1. **Never a markdown table.** GFM always renders a header row, so a table header — whether
   labelled (`| Field | Value |`, which reads like a database dump) or blanked (`|  |  |`, which
   renders as an ugly empty grey bar) — is wrong every time. There is no table form that avoids
   the header row.
2. **Never a single `·`-separated line** (`Status: … · Date: … · Author: …`). It reads as a dense
   wall.
3. **Never a lone bold label with the value indented on the line below.** That doubles the
   vertical whitespace and splits the label from its value, so it looks empty and disjointed. The
   label and its value share one line.

## Diagrams

Do not use a diagram the reader's viewer cannot render. If a ` ```mermaid ` fence shows as raw
code rather than a rendered diagram in the reader's markdown viewer, do not use mermaid — write
the flow as a numbered or bulleted plain-text list, using `→` and indentation. Plain-text flow
renders identically in every viewer. Confirm once how the reader views these files and carry the
answer in the project's convention.

## Titles and headings

Write headings in the artifact's own prose language, not half-and-half. Translate a word that has
a natural equivalent in that language; keep a technical term that developers deliberately never
translate — `registry`, `index`, `gate`, `slice`, `contract`, `drop-in`, `port`, `worktree`,
`monorepo`, `cache`, `metadata` — plus code identifiers, IDs, and paths. The test: a natural
target-language word exists → translate; well-known-in-English jargon → keep. Over-translating
jargon reads as wrong terminology.

Do not append a redundant hyphenated gloss to a heading (`## Brief (pre-work)`); pick one language
for the title and stop.

## Requirements and prose

- Number requirements (`FR-01`, `FR-02`, …) so tasks and reviews can cite them.
- No colloquialisms or spoken-idiom in a delivered artifact. Keep it plain and precise.
