# Finding schema — shared across every tier of znf:review

Every tier (T1/T2/T3) and every capability (M4b finding-verifier, M4e learning-capture)
returns findings in the SAME shape. This is the single schema source — do not redefine it elsewhere.

## Finding

- `dimension` (string): `bugs | security | perf | contracts | intent`
  - `intent` — reads the spec, plan or root cause the ship-pack `## Intent` names and reports intent not implemented, scope added beyond it, or behaviour the spec rules out; with no pack, it judges against the user's stated goal.
- `severity` (string): `CRITICAL | HIGH | MEDIUM | LOW`
- `title` (string): short label
- `file` (string): repo-relative path
- `line` (string): the line (or range) the finding anchors to
- `issue` (string): one-sentence description of the bug
- `fix` (string): suggested fix
- `evidence` (string): a **verbatim** quote of ONE line of code the finding points to (the exact line content in the file, WITHOUT the diff's `+`/`-` marker — the verifier matches it against the real file). **Required when `file+line` is present**.

Required: `dimension, severity, title, issue, fix`. `file/line` recommended when a location exists; `evidence` required when `file+line` is present (so `zenify review-verify` can verify the citation).

## Verdict (set by `zenify review-verify`)

- `refuted` (bool): was the finding rejected?
- `reason` (string): why

`zenify review-verify` sets `refuted`/`reason` when a finding's `evidence` does not match the cited file. Only findings that are NOT refuted make it into the report.
