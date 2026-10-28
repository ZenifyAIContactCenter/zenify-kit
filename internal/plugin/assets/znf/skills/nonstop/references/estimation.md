# Estimation — T-shirt sizes that calibrate themselves

Read when: sizing the morning plan, recording a finished task, or writing the end-of-day stats.

## Scale

| Size | Points | Time | Files changed |
|---|---|---|---|
| S=1 | 1 | < 5 min | ≤ 3 |
| M=2 | 2 | 5 – 20 min | ≤ 10 |
| L=4 | 4 | 20 – 60 min | ≤ 30 |
| XL=8 | 8 | > 60 min | > 30, or the task needs a spec → plan |

The scale is agent time: a worker's `duration_ms`, not a person's hours.

The Time column holds the defaults; once enough tasks are measured the learned scale replaces it (see Learned scale).

The size is the higher of the two criteria (time, files). An XL estimate is marked "should split"
in the plan before it is dispatched.

## Learned scale

Run `zenify nonstop stats --json` (in the workspace) at the morning plan and at end of day; use its `scale` to size tasks and its `today`/`week`/`rules` for the stats and the Calibration line — never compute them by hand. Fewer than 20 measured tasks → the defaults.

## Measuring a finished task

- **Time** = the worker's `duration_ms`, which the harness returns when the worker finishes. Time
  spent waiting on the user is not counted: a worker resumed after a user gate adds the
  `duration_ms` of each run.
- **Files** = `git diff --shortstat <base>...HEAD` run in the worker's worktree, with the base ref
  the worker reported.
- **Rounds** = fix and review rounds from the worker's report. Recorded only; not part of the size.
- **Actual** = the size the time and the files give, the higher of the two.

## `estimates.md`

Next to `progress.md`. One line per finished task:

`date · id · repo · kind · est · actual · time · files · rounds · reason for the gap`

`kind` is the task type (fix, feature, review, chore, …). A task cancelled or restarted from scratch
→ `actual -`, excluded from every statistic.

Every task that reaches `done` gets its line in the same Loop wake-up that records it done — the pane flags a done task without one.

Lines dated before 2026-10-29 were sized on the old human scale (S < 30 min …): for calibration, recompute their actual from the time column with this scale; never use their actual column.

## End-of-day stats

- **Hit rate** = lines of the day with est = actual / lines with an actual.
- **Bias** = how many were over-estimated (est > actual) and how many under-estimated.
- **Throughput** = the sum of actual points of the tasks done today.
- Hit rate of this week lower than last week's → one warning line.

## Morning calibration and batch budget

- Read the last ~30 lines of `estimates.md`. For each planned task, filter the same repo + kind;
  when they show a consistent gap, state the rule (e.g. "<repo> fix: estimate one size up") and the
  history lines it rests on.
- **Batch budget** = the median daily throughput, in points (S=1 … XL=8), when there are ≥ 3 days
  of data; fewer than 3 days → 10 tasks (a count, not points).
