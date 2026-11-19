# Ship with `ENGINE=project`

`ENGINE=$(zenify reviewer | cut -d' ' -f1)`. The project's reviewer reads the pushed head and the PR
later adopts that head's result, so the order of steps 5 and 7 changes.

1. **Before step 5**, with steps 2–4 green: `git add -A && PRE=$(git write-tree)`, commit the work
   on the feature branch, then run 7b's `zenify release-note …` (it commits `--allow-empty`, so the
   work must be committed first or it lands in the note-commit), then push. Same tree →
   `[ "$(git rev-parse 'HEAD^{tree}')" = "$PRE" ]` holds: re-stamp steps 2–4 with the new `fp`. It
   does not hold → re-run them.
2. **Step 5**: `Skill(znf:review)` with `BASE`, the pack and `DECLINES` (empty on round 1).
3. **Fix loop**: each round fixes in ONE commit, re-runs the checks the loop names, pushes, and
   reviews again (the project reviews the new head itself; no scoped re-review). A wrong finding →
   `{id, reason}` with the evidence as reason goes into `declines` of the next send instead of a
   `Declined:` line. Still open after round 2: STOP; the commits are already pushed, so
   do not open the PR; report the finding, what was tried and whether it is load-bearing.
4. **Step 7**: skip 7b (done). `HEAD` = the last reviewed head → push nothing; it differs → back to
   step 5. Run `zenify docs sync` once, then `gh pr create` on that head.

Review fell back (`project→agent:` on the board) → the normal flow from there, but 7b does not run
again.
