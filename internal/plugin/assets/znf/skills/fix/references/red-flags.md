<!-- Moved verbatim from fix/SKILL.md § Red flags (token-diet). Read when: about to skip the scout, the gate or /ship, or on the third attempt at the same bug. -->

| Symptom | Action |
|---|---|
| Fix attempt #3 for same bug | Stop; re-read the actual error from logs |
| "Probably caused by X" | Not probably — confirm with evidence |
| No logs visible | Ask user to provide the actual error output |
| "It's a one-line change, skip the scout" | One line can have any number of callers. Line count is not blast radius |
| Deleting a condition/guard you don't understand | `git log -S` it first. That is how the guard that was containing an outage gets removed |
| The fix has grown to 4+ files | Take the escalation door — `/cook`. You are writing a feature with no spec and no per-task review |
| "Gate was clean, so no review needed" | Gate answers reach, not correctness. `/ship` runs regardless |
