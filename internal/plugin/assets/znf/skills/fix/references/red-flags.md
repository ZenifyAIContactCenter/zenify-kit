<!-- Adapted from fix/SKILL.md § Red flags (token-diet). Read when: about to skip the scout, the gate or /ship, or on the third attempt at the same bug. -->

| Symptom | Action |
|---|---|
| Fix attempt #3 for same bug | Stop; re-run the Step 0 red command and re-read its output |
| "Probably caused by X" | Not probably — confirm with evidence |
| No logs visible | Build the Step 0 loop and re-run the red command; no loop → the three-item ask in `feedback-loop.md` § No loop |
| "It's a one-line change, skip the scout" | One line can have any number of callers. Line count is not blast radius |
| Deleting a condition/guard you don't understand | `git log -S` it first. That is how the guard that was containing an outage gets removed |
| The fix has grown to 4+ files | Take the escalation door — `/cook`. You are writing a feature with no spec and no per-task review |
| "Gate was clean, so no review needed" | Gate answers reach, not correctness. `/ship` runs regardless |
