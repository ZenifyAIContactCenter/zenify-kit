<!-- Moved verbatim from run/SKILL.md § Step 4: Run it in a pane, not in this session (W4 slim-skills). Read when: you are about to change the pane geometry. -->

**Why the column and not a band across the bottom.** Both were built and measured at 203×62:

```
column (right)   agent 134x62   ·  three services 69x21, 69x21, 69x20   → 3 fit, no tab
band (bottom)    agent 203x47   ·  two services 102x15, 101x15          → 3rd needs a tab
```

The column wins on the axis that matters most and the one nobody counts: **the agent pane is what
you read continuously**, and 62 rows against 47 is a third more conversation on screen. The service
panes trade width (69 vs 102) for height (21 vs 15) and for all three fitting at once — and for a
dev log you read the tail, so height is what shows you a stack trace.

**The count that decides overflow is panes in the column, not repos in the task.** Panes-in-column
is state you can measure and it corrects itself when one is closed; repos-in-task is a guess made up
front that goes wrong the moment the task grows — and it is the wrong unit anyway, since one
monorepo can want three of these on its own.

This file has now had the layout wrong twice, in opposite directions, and both times from
over-generalising one measurement. First *"a tab rather than a split: a split leaves each dev pane
~62 columns"* — true of a **vertical** split (agent 121 | dev 62) and then applied to splitting in
general. Then a bottom band, which does give each pane full width but takes 15 rows off the agent
and caps the column at two. The measurement that settles it is the one above: build both, read the
geometry, prefer the layout that protects the pane you actually read.
