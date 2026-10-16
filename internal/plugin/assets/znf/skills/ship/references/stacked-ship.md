# Stacked ship — one review per layer, one stack of PRs

Read when: the plan has a `## Stack` block. These steps replace ship steps 2-7; the one-PR flow in
`SKILL.md` stays as written for every other plan. Layer and trunk names follow the stacked-layers
reference of `znf:subagent-driven-development`: layer 1 is the `wt` branch, layer N >= 2 is
`<layer-1 branch>-<N>`, trunk is `baseRef` without `origin/`.

1. **Steps 2-4 run once, on the top layer branch.** Lint, build, tests, the contract gate and
   behavioural verification see the whole stack there; do not repeat them per layer.

2. **Step 5 runs once per layer, i = 1..N, bottom first.**
   - `gh stack checkout <layer i branch>` on a clean tree.
   - `BASE=$(git merge-base <layer i-1 branch> HEAD)`; for layer 1 the lower branch is the trunk.
   - The layer's ship-pack uses `$BASE..<layer i branch>` for `## Diff`; then `Skill(znf:review)`
     with that `BASE`, so the tier follows the layer's own diff.
   - Never run `review-bundle` over the whole stack. A layer whose review returns `too-large` →
     STOP and propose splitting that layer.
   - After layer N, `gh stack checkout <layer N branch>` again.

3. **Fix loop.** A CRITICAL/HIGH finding in layer k: follow the lower-layer fix procedure (step 4)
   of the stacked-layers reference of `znf:subagent-driven-development` — checkout layer k, commit,
   `gh stack rebase --upstack`, conflict → abort and BLOCKED. Then re-review layer k and every layer
   above it. The two-round cap of the one-PR fix loop still applies.

4. **Step 7.**
   - Append one gate-log line per layer: column 2 is `<basename>#L<i>`.
   - `zenify release-note` once for the whole stack.
   - `gh stack submit --auto --open`.
   - For each PR, bottom to top: `gh pr edit <n> --title "<type(scope): ...>" --body-file <file>`,
     title and body per `pr-message-language.md`; the body states `lớp i/N`.
   - A fix after submit: commit per item 3, then `gh stack push`; re-run `gh pr edit` if the title or
     body was overwritten.
   - The board carries one line per layer: `Layer <i>/<N> <branch> — <PR url>`.

5. **Report** every PR URL, bottom to top, and this sentence: merge the stack bottom-up; merging PR N
   also merges every unmerged PR below it. **Never `gh stack merge`, never merge** — merging is the
   user's deploy decision.
