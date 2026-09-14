# PR message language — machine prefix, human-language body

Read this when writing the `gh pr create` title and body, or when unsure which language each half
of a PR (and the commit) should use.

## The rule

A PR carries two audiences in one artifact. Keep them apart:

- **Title prefix — machine.** The `type(scope):` prefix is parsed by `zenify release-report`
  (`internal/release/classify.go`) to classify the changelog. It stays a conventional-commit type in
  English (`feat` / `fix` / `docs` / `perf` / `refactor` / `chore`) whatever the project's language
  is. This is the one machine token in the whole PR.

- **Description + body — human.** Everything a human reads — the description after the colon and the
  **whole body** — is a human-read artifact. Write it in the project's prose language: the same
  language its specs and plans use (see `znf:_shared/artifact-style` and the project's own
  convention). For the zenify team that is Vietnamese. Structure the body as
  **summary · what changed · how verified**, and end it with the repo's attribution line.

- **Commit messages — machine.** They are the machine record and stay English (Conventional
  Commits). The PR is the human-facing half; the commit is not.

## The failure this removes

Do not split the two halves the wrong way. A title whose description is English sitting over a body
in the project's language — or the reverse, an English body under a localized title — is the exact
inconsistency this rule exists to remove. Pick the audience per field, not per PR.
