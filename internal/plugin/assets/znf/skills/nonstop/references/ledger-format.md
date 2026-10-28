Read when: writing or re-entering the nonstop ledger.

The nonstop pane parses the last dated section of `progress.md` line by line. Anything it cannot
parse is shown as malformed, so keep the section machine-readable.

## Task line

Exactly nine fields separated by ` · `, one task per line:
`id · repo · source · issue · size · skill · worker · status · PR/evidence`

- No tables, no `###` sub-headings inside a day section.
- A PR is always the full URL (`https://github.com/<org>/<repo>/pull/<n>`), never `#n`.
- Use `-` for an empty field.
- Keep all 9 fields on one line; a wrapped line is malformed.
- Never write ` · ` inside a field (a 10th field makes the line malformed); use `;` or `,`.

## Questions for the user

Either set the task status to `blocked` and put the question in the last field, or add a line:
`Needs you: <id> — <question>`

## Allowed note lines

`Ruling: <text>`, `Waiting to merge: <text>`, `Skipped: <text>`. Any other free text goes in the
last field of a task line.

## Re-entering a section that has a table or `###`

If the last section contains a table or `###` headings, rewrite it once, before any dispatch:
turn every row into one 9-field line, keeping each id, status and evidence unchanged. Do not
change the meaning of a row while you rewrite it.
