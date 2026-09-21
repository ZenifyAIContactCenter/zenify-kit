---
name: fix
description: Debug and fix a bug. Use when something is broken — auto-fetches logs/stacktrace to ground the diagnosis in real data. Pass a description, a GitHub Actions URL, or nothing (auto-detect recent logs).
argument-hint: "[description | github-actions-url | empty for auto-log]"
allowed-tools: Read Grep Glob Bash Agent WebFetch
---

**Rigid on diagnosis-first.** Never guess the fix before seeing the real error. Tool names:
`znf:_shared/harness-tools` (harness mapping table).

## How much diagnosis

Never skips Step 2 — state the confirmed cause, with evidence, before writing any fix. **Narrow**
(one clear error → one hypothesis) vs **Wide** (unclear/multi-service/systemic → full Step 1,
parallel hypotheses) — criteria in `references/why-diagnosis-first.md`. State which path and why
(one line); if the narrow hypothesis does not survive Step 2 you were on the wide path — run it.

**Isolation (house rule #8): a worktree, always, created before Step 5 — `references/isolation.md`** (fetch → `wt new <slug> --type fix --base <baseRef>`; a bug found mid-task is not a new task).

## Step 0: Ground in real error data

Fixing the wrong thing is the commonest debugging mistake. Before any analysis:

**No argument:**
```bash
# Auto-fetch recent error output: app logs, test output, error files
ls -t *.log 2>/dev/null | head -3
tail -100 <latest-log-file> 2>/dev/null
```
Also run the project's test suite. **Don't assume `npm`** — use `pm` (`pm test`, `pm run build`),
resolved from `packageManager`/lockfile. Read the repo's CLAUDE.md first if it names another
verify command.

**A description:** a symptom to search for; still fetch logs.

**A GitHub Actions URL (`.../actions/runs/...`):**
```bash
gh run view <run-id> --log 2>&1 | grep -A 20 "Error\|FAIL\|error:" | head -100
# Or: gh run view --log-failed
```

Proceed only on **real error data**.

## Step 1: Root cause analysis

Use superpowers `systematic-debugging`: what is the actual error message (exact text from logs),
and what changed recently (`git log --oneline -10`, `git diff HEAD~1`)?

**Round counter.** Every entry into Step 1 for the *same* error or failing test is one round: write
`ROUND: <n>` in the ship-pack `## Intent` block (1 on first entry, +1 each time Step 2 sends you
back here; a different error resets to 1 and says so). ROUND ≥ 2 →
**Think hard before responding.**

**On the Wide path: one investigator agent per hypothesis, all dispatched in ONE message, each told to return ≤ 40 lines (evidence to a file).**
Identify 2-3 independent hypotheses and the evidence that settles each; the model is not yours to
pick:

```bash
ROUTE=$(bash ~/.claude/skills/znf/skills/_shared/scripts/select-route investigator ROUND=$ROUND); IMODEL=$(printf '%s\n' "$ROUTE" | sed -n '1s/^model=//p')
```

Dispatch one `Explore` agent per hypothesis with `model` = `$IMODEL` (`Explore` pins no model);
print `$ROUTE` line 2 on the report. From ROUND 3, also hand each the logs, killed hypotheses +
evidence, and diffs already tried, as file paths. Each gets one hypothesis, its settling evidence,
and: *return the evidence for AND against, plus a verdict — do not fix anything.* Kill hypotheses
with evidence; **ask each agent for its report by name** — a lost report reads like one that found
nothing (`CLAUDE.md §3`, `references/why-diagnosis-first.md`). Record the dispatch, best-effort:

```bash
command -v zenify >/dev/null && command -v jq >/dev/null && jq -nc --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg repo "$(basename "$(git rev-parse --show-toplevel 2>/dev/null)")" --arg branch "$(git branch --show-current 2>/dev/null)" \
  --arg round "$ROUND" --arg model "$IMODEL" --arg strong "${ZNF_STRONG_MODEL:-opus}" \
  '{ts:$ts,repo:$repo,branch:$branch,"site":"investigator",features:{ROUND:$round},gates:(if ($round|tonumber)>=3 then ["ROUND>=3"] elif ($round|tonumber)==2 then ["ROUND=2"] else [] end),model:$model,strong:$strong}' \
  | zenify route-log record 2>/dev/null || true
```

## Step 2: Verify the real cause

Can you reproduce it (run the failing command/test), and does the evidence match the hypothesis?
If you cannot confirm the cause, say so — never guess-fix.

## Step 3: `/scout` — what depends on the code you will change

Cause confirmed; find what else touches it. Call **`Skill(znf:scout)`** **before** editing — after
means seeking permission. **Invoke it as a tool, not an intention:** it and the `Agent(znf:scout)`
it dispatches each leave a named line, and **a missing line is a skipped step**. Same for
`Skill(znf:ground)` in Step 5.

Brief it on the reverse direction:

1. **who calls / reads / writes** what you are changing
2. **which tests cover it**, and which changed behaviour has none
3. **what else is written in the same operation** (queue job, cache, search index)
4. **why these lines exist** — `git blame -L`, `git log -S`

Target 4 carries the weight (`references/why-diagnosis-first.md`); "cannot enumerate by grep"
means a partial map — carry it forward.

## Step 4: Choose the approach — then check it still belongs here

A confirmed cause leaves more than one way to fix it: symptom vs root cause, caller vs callee,
guard vs restructure, fix forward vs revert. **State the approach in one line with the rejected
alternative and why** — even when obvious; on a real trade-off, put the options to the user and
wait. Goes into the ship-pack `## Intent` block alongside the root cause.

**Escalation door.** More than ~3 files, a cross-service contract, or a design decision instead of
an obvious choice: **stop, run `/cook`.**

## Step 5: Minimal fix

Make the **smallest change** that fixes the confirmed cause, along the Step 4 approach. Do not
refactor unrelated code.

If the fix touches an unverified-this-session shape — DB field, API payload, queue message,
library signature, in-repo symbol, env var — call **`Skill(znf:ground)`** first, so the step
leaves a line.

## Step 6: Verify the fix

Re-run whatever was failing with the project's verify command (not assumed `npm` — see Step 0);
confirm it passes and run related tests.

**This step and `/ship` step 4 are not the same check.** Here the question is narrow: *did the bug
go away*. If it was visual, **look at it in the browser here**, but leave layout measurement and
the screenshot audit to step 4 (`references/why-diagnosis-first.md`).

With **no tests**, "it compiles" is not verification — **`Skill(znf:run)`** to launch the app,
observe the real code path and show that output; invoke it as a tool — it reads the port `wt`
allocated for this worktree.

**A clean check is not yet evidence.** A *negative* result — "no match", "0 results", "OK" —
counts once a second mechanism agrees (`references/clean-check-confirmations.md`).

## Step 7: Report

```
Root cause: <specific cause, with the real error output that confirmed it>
Approach:   <chosen, and what was rejected>
Scout:      <blast radius; uncovered paths; confidence>
Fix:        <what changed and why>
Verified:   PASS / FAIL (at which fingerprint)
```

After Step 8, **`cat` `/ship`'s board file** (`${TMPDIR:-/tmp}/ship-board-<fp10>.md`) below — never
retyped, never collapsed to *"`/ship`: ✅"* (`references/why-diagnosis-first.md`).

## Step 8: Gate, then ship — both, always

**Run `/gate`** (the contract gate): cheap, read-only, safe unprompted. It names the repos reached
and fixes the deploy order — do not judge that by eye.

**Then run `/ship`. Always — not only on cross-repo impact.** One session verifying its own fix is
single-control; `/ship`'s reviewer is the independent party (`references/why-diagnosis-first.md`).
All green, it commits and pushes the branch (rule #7); never on a deploy branch — git-guard
enforces that.

## References

Read one when its trigger fires.

- `references/why-diagnosis-first.md` — read when tempted to skip a step, or wondering why `/fix` insists on scout and ship.
- `references/clean-check-confirmations.md` — read when a negative result is letting you proceed.
- `references/red-flags.md` — read when a symptom keeps repeating (attempt #3, "probably X", 4 files).
