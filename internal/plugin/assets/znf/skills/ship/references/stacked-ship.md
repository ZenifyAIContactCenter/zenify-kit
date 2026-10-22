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
   - `BASE=$(git merge-base <layer i-1 branch> HEAD)`; for layer 1,
     `BASE=$(git merge-base "$(zenify wt base)" HEAD)` with the `origin/...` ref as in ship step 5, never the
     local trunk branch, which may be stale.
   - The layer's ship-pack uses `$BASE..<layer i branch>` for `## Diff`; then `Skill(znf:review)`
     with that `BASE`, so the tier follows the layer's own diff.
   - The layer's `## Deferred` keeps only the `Task <N>:` deferred and parked lines whose task has
     `**Layer:** i` in the plan; a line with no task number goes into the top layer's pack only.
   - Never run `review-bundle` over the whole stack. A layer whose review returns `too-large` →
     STOP and propose splitting that layer.
   - After layer N, `gh stack checkout <layer N branch>` again.

3. **Fix loop.** A fix that belongs to layer k follows the lower-layer fix procedure (step 4) of
   the stacked-layers reference of `znf:subagent-driven-development` — checkout layer k, commit,
   `gh stack rebase --upstack`, conflict → abort and BLOCKED. Any failing check (lint, build, test,
   contract gate, behaviour or a CRITICAL/HIGH finding), and any lower-layer fix, re-enters the
   one-PR fix loop: recompute `fp` on the top layer and re-run steps 2-4 per its rules, then
   re-review layer k and every layer above it. The two-round cap still applies.

4. **Step 7, in this order. Nothing is pushed until (b) is all ✅.**
   - (a) Append ONE gate-log line for the ship run with step 7's `printf`, as in the one-PR flow:
     column 2 is the plain basename and `fp` is the top layer's. Read it back (`tail -30`); if this
     gate has never blocked, say so.
   - (b) Fill the board as in the one-PR flow (every check stamped by its `fp`, `Shippable: YES` only
     if every stamp equals the current one) plus one line per layer, bottom first:
     `Layer <i>/<N> <branch> — <pending>`. Run `zenify ui-verify check` as in step 7 (non-zero → BLOCKED). Write
     the board to `ship-board-$(fp).md` and `cat` it. Any ❌ → STOP, push nothing.
   - (c) `zenify release-note` once for the whole stack.
   - (d) `gh stack submit --auto --open`. This replaces step 7's commit-and-push and its
     `gh pr create`; never run `gh pr create` in a stacked ship.
   - (e) For each PR, bottom to top: `gh pr edit <n> --title "<title>" --body-file <file>`, the title
     taken from the plan's `## Stack` line for layer i, the body per `pr-message-language.md` and
     stating `lớp i/N`.
   - (f) Replace each `<pending>` on the board with that layer's PR URL and `cat` the board again.
   - (g) `zenify docs sync` (fails open).
   - A fix after submit: commit per item 3, then `gh stack push`; re-run `gh pr edit` if the title or
     body was overwritten. Step 6 (deploy order) is unchanged.

5. **Report** every PR URL, bottom to top, and this sentence: merge the stack bottom-up; merging PR N
   also merges every unmerged PR below it. **Never `gh stack merge`, never merge** — merging is the
   user's deploy decision.
