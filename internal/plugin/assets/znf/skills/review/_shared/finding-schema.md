# Finding schema — shared across every tier of znf:review

Every tier (T1/T2/T3) and every capability (M4b finding-verifier, M4e learning-capture)
returns findings in the SAME shape. This is the single schema source — do not redefine it elsewhere.

## Finding

- `dimension` (string): `bugs | security | perf | contracts | types`
- `severity` (string): `CRITICAL | HIGH | MEDIUM | LOW`
- `title` (string): short label
- `file` (string): repo-relative path
- `line` (string): the line (or range) the finding anchors to
- `issue` (string): one-sentence description of the bug
- `fix` (string): suggested fix
- `evidence` (string): a **verbatim** quote of ONE line of code the finding points to (the exact line content in the file, WITHOUT the diff's `+`/`-` marker — the verifier matches it against the real file). **Required when `file+line` is present**.

Required: `dimension, severity, title, issue, fix`. `file/line` recommended when a location exists; `evidence` required when `file+line` is present (so `zenify review-verify` can verify the citation).

## Verdict (adversarial verify — T3, and later M4b finding-verifier)

- `refuted` (bool): was the finding rejected?
- `reason` (string): why

Only findings that are NOT refuted (confirmed by enough skeptics in T3) make it into the report.
</content>
