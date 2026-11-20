# Ship with `ENGINE=project`

`ENGINE=$(zenify reviewer | cut -d' ' -f1)`. The project's reviewer reads the pushed head and the PR
later adopts that head's result, so the order of steps 5 and 7 changes.

1. **Before step 5**, with steps 2–4 green: first, `BRANCH` is one the project engine refuses
   (empty/detached, the base, or a deploy branch, as in `znf:review` `project-engine.md` step 3) →
   commit and push nothing here, record `project→agent: <reason>` on the board, and ship runs with
   `ENGINE=agent` from step 5 on, with its normal 7b. Else, in separate Bash calls: `git add -A`, commit the work on
   the feature branch, run 7b's `zenify release-note …` (it commits `--allow-empty`, so the work
   must be committed first or it lands in the note-commit), then push. `git status --porcelain`
   non-empty after the commit (a hook rewrote files), or the commit hook changed the tree → re-run
   steps 2–4, then commit those changes and push before step 5; else re-stamp steps 2–4 with the
   new `fp`. A commit changes `fp`, so step 4's UI evidence no longer matches it: run step 7's
   `zenify ui-verify check` before each commit (here and in every fix round). Non-zero → handle it
   as step 7 says (`❌ BLOCKED` line, gate-log `BLOCKED`, `ZNF_AUTONOMOUS=1` exit 8 ends the
   session) and commit nothing. Its board line keeps that pre-commit `fp`; step 7's "every
   fingerprint == the current one" exempts it.
2. **Step 5**: `Skill(znf:review)` with `BASE`, the pack and `DECLINES` (empty on round 1).
3. **Fix loop**: each round fixes in ONE commit, re-runs the checks the loop names, pushes, and
   reviews again (the project reviews the new head itself; no scoped re-review). A wrong finding →
   `{id, reason}` with the evidence as reason goes into `declines` of the next send instead of a
   `Declined:` line; a wrong mechanical (PRE) finding has no `id`, so it gets ship's `Declined:` board line instead. Still open after round 2: STOP; the commits are already pushed, so
   do not open the PR; report the finding, what was tried and whether it is load-bearing.
4. **Step 7**: skip 7b (done) and the `ui-verify check` (run before each commit, also after a fallback). `HEAD` = the last reviewed head → push nothing; it differs → back to
   step 5 (that counts as a round). Run `zenify docs sync` once, then `gh pr create` on that head.

Review fell back (`project→agent:` on the board) in any round, with `ENGINE=project` → the rest of this ship uses
`ENGINE=agent` (every later `Skill(znf:review)` gets `engine agent` in its args), the normal flow from there, and 7b does not run again. A
fallback round counts as a fix-loop round.

**Exception to ship's "Any ❌ → fix, no commit" and "On all-green, commit and push":** with
`ENGINE=project` the commits and push happen before review, so red after round 2 leaves pushed
commits and opens no PR.

## `ENGINE=agent+project`

The Claude review first, then the project's on the same branch; both use step 5's `BASE`.

1. **Claude pass**: step 5 with `engine agent` in `Skill(znf:review)`'s args, and ship's own fix loop
   (scoped re-review, 2 rounds; still open after round 2 → STOP, commit nothing), exactly as `agent`.
2. **Project pass**, once the Claude pass is clean: step 1 above (ui-verify check, commit, 7b, push),
   then steps 2–4 above with `engine project` in every `Skill(znf:review)`'s args. Its fix loop has
   its own 2 rounds; red after round 2 → the exception above.
3. **Project path drops out** (step 1's refused `BRANCH`, or `znf:review` returns
   `project bỏ qua: <reason>`, in any round): never a second full Claude review. Board line
   `review … project bỏ qua: <reason>`. Fix commits from an earlier project round → ONE scoped
   re-review with `engine agent`: the project findings they fixed, verbatim, plus only `git diff <the head the Claude pass last saw>..HEAD`,
   as ship's fix loop; dropped out before any fix → nothing more. The rest of ship runs as `agent` after review: 7b only if 7b has not run,
   then the missing commit or push, then `gh pr create`.
4. **Board**: two review lines, `review … agent: …` and `review … project: …` (or
   `project bỏ qua: <reason>`). `Shippable: YES` only when the agent line is clean and the project
   line is clean or skipped. The agent line keeps its pre-commit `fp`; step 7's "every fingerprint
   == the current one" exempts it, as it does the pre-commit `ui-verify check`.
