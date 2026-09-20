---
name: fix
description: Debug and fix a bug. Use when something is broken — auto-fetches logs/stacktrace to ground the diagnosis in real data. Pass a description, a GitHub Actions URL, or nothing (auto-detect recent logs).
argument-hint: "[description | github-actions-url | empty for auto-log]"
allowed-tools: Read Grep Glob Bash Agent WebFetch
---

**Rigid on diagnosis-first.** Never guess the fix before seeing the real error. Tool names:
`znf:_shared/harness-tools` (harness mapping table).

## How much diagnosis

It never skips a step, least of all Step 2: state the confirmed cause, with evidence, before
writing any fix.


- **Narrow** — one clear error, one obvious cause → one hypothesis, checked against the real
  error output, then Step 2.
- **Wide** — unclear cause, multiple services, hard to reproduce, or systemic → Step 1 in full,
  parallel hypotheses, each killed or kept by specific evidence.

State which path and why (one line). If the narrow hypothesis does not survive Step 2 you were on
the wide path all along — run it.

**Isolation (house rule #8): a worktree, always — whatever the size of the fix.**

> **Isolation & base-ref doctrine → znf:discipline §8** (single source): worktree unconditional, base = the declared baseRef, fetch before resolving it. Below is `/fix`'s operational step only.

```bash
git -C <repo> fetch origin                                   # before resolving the base — see znf:discipline §8
cd <repo> && wt new <slug> --type fix --base "$(node -e 'console.log(JSON.parse(require("fs").readFileSync(".claude/worktree.json","utf8")).baseRef)')"
```

One worktree per affected repo, same slug. No workspace handoff here, unlike `/cook`;
`Skill(znf:run)` still gives the repo a `dev` pane when the fix needs it running.

**A bug found mid-task is not a new task** — `cd` into the worktree that exists. Only unrelated
work earns `--another`; a live production bug is `/hotfix`. Do this **before Step 5**, not Step 0
(`references/why-diagnosis-first.md`).

## Step 0: Ground in real error data

Fixing the wrong thing is the commonest debugging mistake. Before any analysis:

**No argument:**
```bash
# Auto-fetch recent error output: app logs, test output, error files
ls -t *.log 2>/dev/null | head -3
tail -100 <latest-log-file> 2>/dev/null
```
Also run the project's test suite. **Don't assume `npm`** — use `pm` (`pm test`, `pm run build`),
which resolves the manager from `packageManager` then the lockfile. Read the repo's CLAUDE.md
first if it names another verify command.

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



**On the Wide path: one investigator agent per hypothesis, all dispatched in ONE message, each told to return ≤ 40 lines (evidence to a file).**
Identify 2-3 independent hypotheses and the evidence that would settle each. Dispatch one `Explore`
agent per hypothesis — **all in a single message**, which makes them concurrent — and **name
`model: 'sonnet'` on each** (`Explore` pins no model). Give each one hypothesis, its settling
evidence, and: *return the evidence found for AND against, plus a verdict — do not fix anything.*
Kill hypotheses with evidence, and **ask each agent for its report by name**: a lost report reads
like one that found nothing (`CLAUDE.md §3`, `references/why-diagnosis-first.md`).

## Step 2: Verify the real cause

Can you reproduce it (run the failing command/test), and does the evidence match the hypothesis?
If you cannot confirm the cause, say so — never guess-fix.

## Step 3: `/scout` — what depends on the code you will change

The cause is confirmed; now find what else touches it. Call **`Skill(znf:scout)`** **before**
editing — after means the change is made and you are seeking permission. **Invoke it as a tool, not
an intention:** `Skill(znf:scout)` and the `Agent(znf:scout)` it dispatches each leave a named line,
and **a missing line is a skipped step**. Same for `Skill(znf:ground)` in Step 5.

Brief it on the reverse direction:

1. **who calls / reads / writes** what you are changing
2. **which tests cover it**, and which changed behaviour has none
3. **what else is written in the same operation** (queue job, cache, search index)
4. **why these lines exist** — `git blame -L`, `git log -S`

Target 4 carries the weight (`references/why-diagnosis-first.md`). **"Cannot enumerate by grep"**
means a partial map — carry that forward.

## Step 4: Choose the approach — then check it still belongs here

A confirmed cause leaves more than one way to fix it: symptom vs root cause, caller vs callee,
guard vs restructure, fix forward vs revert. **State the approach in one line with the alternative
you rejected and why** — even when obvious. On a real trade-off, put the options to the user and
wait. It goes into the ship-pack `## Intent` block alongside the root cause.

**Escalation door.** If the fix spans more than ~3 files, changes a cross-service contract, or
needs a design decision rather than a choice between obvious options: **stop, run `/cook`.**

## Step 5: Minimal fix

Make the **smallest change** that fixes the confirmed cause, along the Step 4 approach. Do not
refactor unrelated code.

If the fix touches a shape you haven't verified this session — DB field, API request/response, queue payload, library signature, in-repo symbol, env var — call **`Skill(znf:ground)`** first, so the step leaves a line.

## Step 6: Verify the fix

Re-run whatever was failing with the project's own verify command (not assumed to be `npm` — see Step 0). Confirm it passes, and run related tests.

**This step and `/ship` step 4 are not the same check.** Here the question is narrow: *did the bug
go away*. If it was visual, **look at it in the browser here**, but leave layout measurement and
the screenshot audit to step 4 (`references/why-diagnosis-first.md`).

With **no tests**, "it compiles" is not verification — **`Skill(znf:run)`** to launch the app, observe the real code path and show that output. Invoke it as a tool rather than by hand: it reads the port `wt` allocated for this worktree.

**A clean check is not yet evidence.** A *negative* result — "no match", "0 results", "OK" — that
lets you **proceed** counts only once a second mechanism agrees
(`references/clean-check-confirmations.md`).

## Step 7: Report

```
Root cause: <specific cause, with the real error output that confirmed it>
Approach:   <chosen, and what was rejected>
Scout:      <blast radius; uncovered paths; confidence>
Fix:        <what changed and why>
Verified:   PASS / FAIL (at which fingerprint)
```

After Step 8, **`cat` `/ship`'s board file** (`${TMPDIR:-/tmp}/ship-board-<fp10>.md`) underneath
this — never retyped, never collapsed to *"`/ship`: ✅"* (`references/why-diagnosis-first.md`).

## Step 8: Gate, then ship — both, always

**Run `/gate`** (the contract gate): cheap, read-only, safe unprompted. It tells you which repos
the change reaches and fixes the deploy order — do not judge that by eye.

**Then run `/ship`. Always — not only when the gate reports cross-repo impact.** A fix written and
verified by one session is a single-control setup; `/ship`'s reviewer is the independent party
(`references/why-diagnosis-first.md`). All green, it commits and pushes the branch itself (rule
#7). Never commit on a deploy branch — git-guard enforces that.

## References

Read one when its trigger fires.

- `references/why-diagnosis-first.md` — read when tempted to skip a step, or wondering why `/fix` insists on scout and ship.
- `references/clean-check-confirmations.md` — read when a negative result is letting you proceed.
- `references/red-flags.md` — read when a symptom keeps repeating (attempt #3, "probably X", 4 files).
