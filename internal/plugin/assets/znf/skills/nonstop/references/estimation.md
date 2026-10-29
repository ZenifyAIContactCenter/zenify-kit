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

Run `zenify nonstop stats --json --dir <the ledger directory>` (the directory holding progress.md) at the morning plan and at end of day; use its `scale` to size tasks and its `today`/`week`/`rules` for the stats and the Calibration line — never compute them by hand. Fewer than 20 measured tasks → the defaults.

## Measuring a finished task

- **Time** = the sum of the task's runs in `durations.log`. Time spent waiting on the user is not
  counted: a worker resumed after a user gate adds each run.
  `durations.log` sits next to `estimates.md`: the pane mod appends one line per
  worker run (`work`) and per reviewer run (`review`) and `zenify nonstop stats` sums them per
  task. The orchestrator no longer writes times by hand; `-` in the work and review columns is fine.
- **Files** = `git diff --shortstat <base>...HEAD` run in the worker's worktree, with the base ref
  the worker reported.
- **Rounds** = fix and review rounds from the worker's report. Recorded only; not part of the size.
- **Actual** = the size the time and the files give, the higher of the two.
- **Tier** = line 1 of `znf:review`'s select-tier output for that task's review (`T1`, `T2` or `T3`;
  `-` when the task had no review).
- **Path** = the brainstorming path the worker reports (`spike`, `bounded` or `architectural`; `-` for
  routes without brainstorming).

## `estimates.md`

Next to `progress.md`. One line per finished task:

`date · id · repo · kind · est · actual · work · review · files · rounds · path · tier · reason for the gap`

A line has 13 fields. v1 lines still read: the old 10-field `date · id · repo · kind · est · actual · time · files · rounds · reason`
line counts its time as work, with no review, path or tier.

`kind` is the task type (fix, feature, review, chore, …). A task cancelled or restarted from scratch
→ `actual -`, excluded from every statistic.

Every task that reaches `done` gets its line in the same Loop wake-up that records it done — the pane flags a done task without one.

Lines dated before 2026-10-29 were sized on the old human scale (S < 30 min …): for calibration, recompute their actual from the time column with this scale; never use their actual column.

## End-of-day stats

- **Hit rate** = lines of the day with est = actual / lines with an actual.
- **Bias** = how many were over-estimated (est > actual) and how many under-estimated.
- **Throughput** = the sum of actual points of the tasks done today.
- Hit rate of this week lower than last week's → one warning line.
- Take the numbers from `zenify nonstop stats --json --dir <the ledger directory>` (`today`, `week`, `tiers`), never by hand.

## Morning calibration and batch budget

Take the numbers from `zenify nonstop stats --json --dir <the ledger directory>` (`scale`, `rules`, `week`) and write its `scale` line
into the plan's Calibration line (FR-5.7).

- Read the last ~30 lines of `estimates.md`. For each planned task, filter the same repo + kind;
  when they show a consistent gap, state the rule (e.g. "<repo> fix: estimate one size up") and the
  history lines it rests on.
- **Batch budget** = the median daily throughput, in points (S=1 … XL=8), when there are ≥ 3 days
  of data; fewer than 3 days → 10 tasks (a count, not points).
