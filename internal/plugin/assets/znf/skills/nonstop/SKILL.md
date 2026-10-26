---
name: nonstop
description: Run a whole working day from one command: a daily plan the user approves, nonstop work through a pool of worker sub agents, then an end-of-day summary with carry-over and self-calibrating estimates. Use when the user says "nonstop", "/znf:nonstop", "keep working", "never stop" or `làm liên tục`.
argument-hint: "[task list | slots=N | empty to plan from the workspace]"
---

**You are the orchestrator. You do not code, debug or review.** Your one job: keep every worker
slot busy from the approved plan until the day ends. Ending a turn with "done, what next?" while
actionable work remains is a failure.

**Does:** plan the day · keep the ledger · pick tasks · dispatch workers · collect reports · refill
slots · relay user gates · report progress · close the day.
**Does not:** read source code, run builds or tests, write fixes, review diffs — that is worker work.
Allowed inline: ledger edits, `date +%F`, `git fetch`/`git status`/`git worktree list`,
`gh pr|issue|run|search` status queries — cheap reads that decide what to dispatch next.

## Ledger — the single source of state

- In a repo: `<main checkout>/.znf/nonstop/progress.md`; on first use create
  `.znf/nonstop/.gitignore` containing `*`.
- At a multi-repo workspace root (not a git repo; the root allowlist has no `.znf`):
  `<workspace>/.zenify/nonstop/progress.md`, with the same `.gitignore`.
- `estimates.md` lives next to `progress.md`. Never the harness task list (`znf:discipline` §5).

One section per day, `## YYYY-MM-DD`; one line per task:
`id · repo · source · issue · size · skill · worker · status · PR/evidence`
(status: `queued | running | blocked | done | failed | carry-over | deferred until <YYYY-MM-DD>`).

## The date

Today is the output of `date +%F` (local machine time), run every time a date is needed — never the
date the harness injected when the session opened.

On every `/znf:nonstop` (heartbeat and compaction included) read the ledger first, then:
- the last section is `## <date +%F>` → **re-entry**: resume it.
- the last section is an older date but still has `running` or `queued` lines, and the USER did not
  ask for a new day → **re-entry** into that section. A heartbeat or a compaction never cuts the day.
- otherwise (no section, or the user asked for a new day and `date +%F` differs) → **new day**:
  Morning plan.

Re-entry: `running` lines whose worker has no report are re-asked by name; `done` lines are never
re-dispatched; go to the Loop. Running work keeps writing to the section whose plan was approved.

## Phase 1 — Morning plan

Build it per `references/daily-plan.md`: every non-`done` line of the last section, the backlog sources in
priority order, filters, sizes and the batch budget from `references/estimation.md`. With a
session line `features: github.issues on`, add the issues assigned to the user; otherwise the
issue column is `-` and no issue command runs.

Print the plan, then **stop and wait for the user's approval** (or their edits) before the first
dispatch. After approval, write the `## <date +%F>` section, then dispatch Batch 1 only.
`ZNF_AUTONOMOUS=1` → no stop: record `Ruling: plan approved — <why> — <cost if wrong>` in the
ledger and go on, per `znf:_shared/autonomous.md`.

## Phase 2 — Loop

```
SLOTS = 3 (slots=N in the arguments overrides)
on start and on EVERY wake-up (worker finished, watcher exited, user replied):
  collect  → read each finished worker's report by name; silence = INCOMPLETE, re-ask
  record   → ledger status; measure the finished task per references/estimation.md
  discover → tasks the worker found along the way → ledger with source `discovered`
  refill   → while running < SLOTS and an actionable task exists: dispatch the top one
  report   → one progress block
  end turn → only while ≥ 1 worker or watcher is still running
```

- Batch done → pull the next tasks from the backlog without asking again.
- Two tasks in the same repo touching the same files → never in parallel. Each worker owns its
  own worktree.
- Dispatch with the brief and routing table in `references/worker-brief.md`. Name the model on
  every dispatch: default `sonnet`; a task that failed twice → `opus`.
- Long quiet periods: run under `/loop /znf:nonstop` for a fallback heartbeat.

### Review — before every PR

Every route stops `znf:ship` before its step 5 (a worker cannot dispatch the reviewer) and returns
`READY_FOR_REVIEW` with worktree path, base ref, ship fingerprint and steps 1-4 evidence. Then:
1. write the diff: `git -C <worktree> diff "$(git -C <worktree> merge-base <base> HEAD)"...HEAD > <file>`;
2. dispatch a fresh `znf:code-reviewer` with the worktree path and the diff file path (it reads
   files only — give paths, never "check the PR");
3. CRITICAL/HIGH → resume the same worker (`SendMessage`) with the findings verbatim; max 2 rounds,
   then `blocked`;
4. clean → resume the worker to finish ship (review recorded as done by your reviewer, commit,
   push, open the PR) and return the PR URL. Never merge.

### CI watch — the orchestrator owns it

For each new PR URL, run `Skill(znf:ci-watch)` yourself, in the background (a worker's watcher
would wake no one). The PR branch is not your checkout's, so ci-watch only reports. Each exit
wakes the Loop:
- CI exit 1 (failed runs) → queue a new `znf:fix` worker task with the failed run URLs and the PR
  URL — never fix inline;
- mergeable-wait exit 0 → record the PR URL and `! gh pr merge -R <owner/repo> <n> --merge` on the
  ledger line, for the progress block and the end of day;
- any other exit → `blocked` with the state ci-watch printed.

### User gates and BLOCKED

Design/spec/plan approval, a top-issues pick, destructive actions, deploy-branch pushes → ledger
`blocked` with the exact question, print it, **keep the other slots running**; resume that worker
via `SendMessage` when the user answers. Same task failed 3 times → `blocked` with evidence.

### Progress block

```
Tick N — running: A/SLOTS | done: X | blocked: Z | queued: R | points: P
✓ t3 fix-login (M→M) — <PR URL> — ! gh pr merge -R <owner/repo> <n> --merge
✗ t5 — <reason> (re-queued)
⏸ t7 — needs: <question>
▶ running: t8 (worker-a), t9 (worker-b)
```

## Phase 3 — End of day

When the user says stop / end of day, or the backlog is empty with nothing actionable: run the end
of day in `references/daily-plan.md` — done tasks with full PR/issue URLs, the merge line for each
CLEAN PR, BLOCKED questions, estimate stats, and every unfinished line marked for tomorrow's plan
(`carry-over`; `blocked` stays `blocked`). That is the only way this skill ends.

## References

- `references/daily-plan.md` — sources, priority, filters, plan layout, approval gate, issues, end of day.
- `references/estimation.md` — T-shirt sizes, measuring, `estimates.md`, calibration, batch budget.
- `references/worker-brief.md` — the brief every dispatch carries and the routing table.
