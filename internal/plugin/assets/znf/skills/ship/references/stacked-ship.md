# Stacked ship — one review per layer, one stack of PRs

Read when: the plan has a `## Stack` block. These steps replace only the parts of ship steps 2-7
named below; everything else in those steps still applies once, on the top layer. The one-PR flow
in `SKILL.md` stays as written for every other plan. Layer and trunk names follow the stacked-layers
reference of `znf:subagent-driven-development`: layer 1 is the `wt` branch, layer N >= 2 is
`<layer-1 branch>-<N>`. Trunk is `baseRef` without `origin/`, and only `gh stack` commands take it
in that stripped form; git commands use `<baseRef>` itself.

1. **Steps 2-4 run once, on the top layer branch.** Lint, build, tests, the contract gate and
   behavioural verification see the whole stack there; do not repeat them per layer.

2. **Step 5 runs once per layer, i = 1..N, bottom first.**
   - `gh stack checkout <layer i branch>` on a clean tree.
   - `BASE=$(git merge-base <layer i-1 branch> HEAD)`; for layer 1 use `<baseRef>` (the `origin/...`
     ref, as in ship step 5), never the local trunk branch, which may be stale.
   - The layer's ship-pack uses `$BASE..<layer i branch>` for `## Diff`; then `Skill(znf:review)`
     with that `BASE`, so the tier follows the layer's own diff.
   - Never run `review-bundle` over the whole stack. A layer whose review returns `too-large` →
     STOP and propose splitting that layer.
   - After layer N, `gh stack checkout <layer N branch>` again.

3. **Fix loop.** A fix that belongs to layer k follows the lower-layer fix procedure (step 4) of
   the stacked-layers reference of `znf:subagent-driven-development` — checkout layer k, commit,
   `gh stack rebase --upstack`, conflict → abort and BLOCKED. Any failing check (lint, build, test,
   contract gate, behaviour or a CRITICAL/HIGH finding), and any lower-layer fix, re-enters the
   one-PR fix loop: recompute `fp` on the top layer and re-run steps 2-4 per its rules, then
   re-review layer k and every layer above it. The two-round cap still applies.

4. **Step 7.**
   - Append one gate-log line per layer with ship step 7's `printf`: column 2 is `<basename>#L<i>`,
     and `fp` is computed with that layer's branch checked out.
   - `zenify release-note` once for the whole stack.
   - `gh stack submit --auto --open`.
   - For each PR, bottom to top: `gh pr edit <n> --title "<type(scope): ...>" --body-file <file>`,
     title and body per `pr-message-language.md`; the body states `lớp i/N`.
   - A fix after submit: commit per item 3, then `gh stack push`; re-run `gh pr edit` if the title or
     body was overwritten.
   - The board carries one line per layer: `Layer <i>/<N> <branch> — <PR url>`.
   - Everything else in ship step 7 still applies once, on the top layer: the gate-log read-back
     (if this gate has never blocked, say so), the board with every check stamped by its `fp` and
     `Shippable: YES` only if every stamp equals the current one, written to
     `ship-board-$(fp).md` and `cat`ed, `zenify ui-verify check` (non-zero → BLOCKED), and
     `zenify docs sync` after the push. Step 6 (deploy order) is unchanged.

5. **Report** every PR URL, bottom to top, and this sentence: merge the stack bottom-up; merging PR N
   also merges every unmerged PR below it. **Never `gh stack merge`, never merge** — merging is the
   user's deploy decision.
