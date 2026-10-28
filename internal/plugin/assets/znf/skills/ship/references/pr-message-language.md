# PR message language — machine prefix, human-language body

Read this when writing the `gh pr create` title and body, or when unsure which language each half
of a PR (and the commit) should use.

## The rule

A PR carries two audiences in one artifact. Keep them apart:

- **Title prefix — machine.** The `type(scope):` prefix is parsed by `zenify release-report`
  (`internal/release/classify.go`) to classify the changelog. It stays a conventional-commit type in
  English (`feat` / `fix` / `docs` / `perf` / `refactor` / `chore`) whatever the project's language
  is. This is the one machine token in the whole PR.

- **Tracker key — machine.** If the current branch name matches `(^|[/-])zen-(\d+)(-|$)`
  (case-insensitive, at a slug boundary, for example `u/fix/zen-123-foo`; `frozen-12-x`, `citizen-3` and
  `dozen-5` do not match; if several match, the first wins), the title becomes `<type>(<scope>): ZEN-<n> <description>` with the key
  in uppercase, right after the colon. A branch without a key keeps the title unchanged. In a stack,
  layer N>=2 branches are `<layer-1>-<N>`, so the key stays in every branch name. In a stacked plan
  (`## Stack`) every layer except the top one also gets the body line `Part of ZEN-<n>`, placed right
  before the attribution line; the top layer does not carry it.

- **Description + body — human.** Everything a human reads — the description after the colon and the
  **whole body** — is a human-read artifact. Write it in the project's prose language: the same
  language its specs and plans use (see `znf:_shared/artifact-style` and the project's own
  convention). For the zenify team that is Vietnamese. Structure the body as
  **summary · what changed · how verified · merge risk**, and end it with the repo's attribution line.

- **Commit messages — machine.** They are the machine record and stay English (Conventional
  Commits). The PR is the human-facing half; the commit is not.

## The failure this removes

Do not split the two halves the wrong way. A title whose description is English sitting over a body
in the project's language — or the reverse, an English body under a localized title — is the exact
inconsistency this rule exists to remove. Pick the audience per field, not per PR.

## Body structure

The body is, in this order: **summary · what changed · how verified · merge risk**, then the repo's
attribution line. Write it for the human reviewer: no list of spec `SC-n` lines, no test function names, no
SDD ledger lines (`minor (deferred)` / `parked`) — the reviewer cannot act on any of them, and the
ship board and ledger already hold them. Name in plain words only what was NOT verified and what a
person still has to decide or do by hand.

With `github.issues on` in the session or in the task brief's `Features:` field, follow
`github-issues.md` before `gh pr create` (find or create the issue); the body then carries a
`Closes #N` line right before the attribution line.

### Merge risk section

- Heading and prose follow the project's prose language, like the rest of the body.
- State whether the PR is one-way or two-way, its blast radius (from the `_Blast-radius` tag in the
  spec's Brief) and its rollback (from `_Rollback`). These are the values ship 7b passes to
  `zenify release-note`.
- No spec: write the blast radius from the files changed.
- Before→after evidence belongs in "how verified"; do not repeat it here.
- One-way means the PR has at least one of: a migration or backfill; a write to a shared data store
  or a schema change; a message sent to end users; a publish of an artifact others download
  (package, SDK, CDN file). Otherwise it is two-way.
