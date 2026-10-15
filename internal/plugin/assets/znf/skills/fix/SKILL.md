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

**Isolation (house rule #8): a worktree, always, created before the loop writes its first file or starts a server, at the latest before Step 5 — `references/isolation.md`** (fetch → `wt new <slug> --type fix --base <baseRef>`; read-only rungs need none; a bug found mid-task is not a new task).

## Step 0: Ground in real error data

**No argument:**
```bash
ls -t *.log 2>/dev/null | head -3
tail -100 <latest-log-file> 2>/dev/null
```
Also run the project's test suite with `pm` (`pm test`, `pm run build`), never an assumed `npm`; a
verify command in the repo's CLAUDE.md wins.

**A description:** a symptom to search for; still fetch logs.

**A GitHub Actions URL (`.../actions/runs/...`):**
```bash
gh run view <run-id> --log 2>&1 | grep -A 20 "Error\|FAIL\|error:" | head -100
# Or: gh run view --log-failed
```

**Prod/staging symptom (any input):** pull evidence in order: Loki, Tempo,
`zenify db-read`/`redis-read`, PostHog — recipes: `references/feedback-loop.md`.

**Build the loop.** Step 0 ends with exactly one named command, already run (show invocation and
output), red on the symptom the user described. Ladder and redaction:
`references/feedback-loop.md`; secrets in logs → `<REDACTED>`. No loop → stop and ask for (a) a
reproducing environment, (b) a redacted artifact, (c) leave to add temporary logs; no Step 1.

## Step 1: Root cause analysis

Start from the evidence: what is the actual error message (exact text from logs),
and what changed recently (`git log --oneline -10`, `git diff HEAD~1`)?

**Round counter.** Every entry into Step 1 for the *same* error or failing test is one round: write
`ROUND: <n>` in the ship-pack `## Intent` block (1 on first entry, +1 each time Step 2 sends you
back here; a different error resets to 1 and says so). ROUND ≥ 2 →
**Think hard before responding.**

**On the Wide path: one investigator agent per hypothesis, all dispatched in ONE message, each told to return ≤ 40 lines (evidence to a file).**
Identify 2-3 independent hypotheses and the evidence that settles each; the model is set by ROUND,
not picked: dispatch one `Explore` agent per hypothesis with ROUND 1 `model: sonnet`, ROUND 2
`model: opus`, ROUND ≥ 3 `model: opus` + `effort: xhigh`, and from ROUND 3 hand each the logs,
killed hypotheses + evidence, and diffs already tried, as file paths. Each gets one hypothesis, its
settling evidence, the Step 0 red command, and: *return the evidence for AND against, plus a
verdict — do not fix anything.*
Kill hypotheses with evidence; **ask each agent for its report by name** — a lost report reads like
one that found nothing (`CLAUDE.md §3`, `references/why-diagnosis-first.md`).

## Step 2: Verify the real cause

Run the Step 0 command: red on the user's symptom, not a neighbouring error? Shrink it to the
smallest scenario still red. Does the evidence match the hypothesis? If you cannot confirm the
cause, say so — never guess-fix.

## Step 3: `/scout` — what depends on the code you will change

Cause confirmed; find what else touches it. Call **`Skill(znf:scout)`** **before** editing — after
means seeking permission. **Invoke it as a tool:** it and the `Agent(znf:scout)` it dispatches
each leave a named line; **a missing line is a skipped step** (same for `Skill(znf:ground)`, Step 5).

Brief it on the reverse direction:

1. **who calls / reads / writes** what you are changing
2. **which tests cover it**, and which changed behaviour has none
3. **what else is written in the same operation** (queue job, cache, search index)
4. **why these lines exist** — `git blame -L`, `git log -S`

Target 4 carries the weight (`references/why-diagnosis-first.md`); "cannot enumerate by grep"
means a partial map — carry it forward.

## Step 4: Choose the approach — then check it still belongs here

Symptom vs root cause, caller vs callee, guard vs restructure, fix forward vs revert. **State the
approach in one line with the rejected alternative and why** — even when obvious; on a real
trade-off, put the options to the user and wait. Goes into the ship-pack `## Intent` block alongside the root cause.

**Escalation door.** More than ~3 files, a cross-service contract, or a design decision instead of
an obvious choice: **stop, run `/cook`.**

## Step 5: Minimal fix

**Regression test first.** With a correct seam (hits the bug as the real caller does): turn the
repro into a test, watch it fail, fix, watch it pass; if red needed a code or fixture change, show
the `diff`. No seam → `no seam: <why>` in the ship-pack `## Intent`. You pick; never ask.

Make the **smallest change** that fixes the confirmed cause, along the Step 4 approach. Do not
refactor unrelated code.

If the fix touches an unverified-this-session shape — DB field, API payload, queue message,
library signature, in-repo symbol, env var — call **`Skill(znf:ground)`** first, so the step
leaves a line.

## Step 6: Verify the fix

Re-run whatever was failing with the project's verify command (not assumed `npm`); confirm it
passes and run related tests. Re-run the Step 0 command on the original (unshrunk) scenario: green. Remove every `[DEBUG-xxxx]` log — `rg '\[DEBUG-'` on changed files and
`git diff <base> | rg '\[DEBUG-'` both return nothing.

**Not `/ship` step 4:** only *did the bug go away*. If visual, **look in the browser here**; layout
measurement and the screenshot audit wait for step 4 (`references/why-diagnosis-first.md`).

With **no tests**, "it compiles" is not verification — **`Skill(znf:run)`** to launch the app,
observe the real code path and show that output; it reads the port `wt` allocated for this worktree.

**A clean check is not yet evidence.** A *negative* result — "no match", "0 results", "OK" —
counts once a second mechanism agrees (`references/clean-check-confirmations.md`).

## Step 7: Report

```
Root cause: <specific cause, with the real error output that confirmed it>
Approach:   <chosen, and what was rejected>
Scout:      <blast radius; uncovered paths; confidence>
Fix:        <what changed and why>
Loop:       <command> — red → green
Regression: <test name> | no seam: <why>
Verified:   PASS / FAIL (at which fingerprint)
```

After Step 8, **`cat` `/ship`'s board file** (`${TMPDIR:-/tmp}/ship-board-<fp10>.md`) below — never
retyped (`references/why-diagnosis-first.md`).

## Step 8: Gate, then ship — both, always

**Run `/gate`** (the contract gate; read-only, safe unprompted): it names the repos reached and
fixes the deploy order — do not judge that by eye.

**Then run `/ship`. Always — not only on cross-repo impact.** `/ship`'s reviewer is the independent
party (`references/why-diagnosis-first.md`). All green, it commits and pushes the branch (rule #7); git-guard blocks deploy branches.

## References

Read one when its trigger fires.

- `references/why-diagnosis-first.md` — read when tempted to skip a step, or wondering why `/fix` insists on scout and ship.
- `references/feedback-loop.md` — read when building the Step 0 loop, redacting logs, or tagging debug output.
- `references/clean-check-confirmations.md` — read when a negative result is letting you proceed.
- `references/red-flags.md` — read when a symptom keeps repeating (attempt #3, "probably X", 4 files).
