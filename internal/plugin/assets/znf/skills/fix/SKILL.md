---
name: fix
description: Debug and fix a bug. Use when something is broken — auto-fetches logs/stacktrace to ground the diagnosis in real data. Pass a description, a GitHub Actions URL, or nothing (auto-detect recent logs).
argument-hint: "[description | github-actions-url | empty for auto-log]"
allowed-tools: Read Grep Glob Bash Agent WebFetch
---

**Rigid on diagnosis-first.** Never guess the fix before seeing the real error.

## How much diagnosis

This scales **how much diagnosis** to run. It never skips a step, and in particular it never
skips Step 2 — you state the confirmed root cause, with the evidence that confirmed it,
before writing any fix, on both paths. "It looked simple" is how you end up fixing the
wrong thing, which Step 0 exists to prevent.

**Narrow** — one clear error message pointing at one obvious cause → one hypothesis, checked
against the real error output, then Step 2.

**Wide** — any of: unclear root cause, multiple services involved, hard to reproduce,
systemic/recurring → Step 1 in full with parallel hypotheses, each killed or kept by
specific evidence.

State which path and why (one line). If the narrow path's single hypothesis does not survive
Step 2, you were on the wide path all along — go run it, do not patch the symptom.

**Isolation (house rule #8): a worktree, always — no conditions, whatever the size of the fix.**
The main checkout stays clean.

```bash
git -C <repo> fetch origin                                   # wt does NOT fetch
cd <repo> && wt new <slug> --type fix --base origin/staging
```

The `fetch` is load-bearing: `wt` contains no `git fetch`, so `--base origin/staging` otherwise
resolves against a local ref that may be weeks old, and you write the fix on top of code that has
already moved. One worktree per affected repo, same slug.

**No workspace handoff here, unlike `/cook` — and the reason is the artifact.** `/cook` can move
Step 6 into a workspace of its own because what crosses over is a **plan file** that
`writing-plans` already requires to be self-contained. `/fix`'s equivalent is the *diagnosis*, and
that lives in this conversation: Step 0-3's evidence, the hypotheses killed, the one that survived.
Hand that to a fresh session and it starts by re-deriving what you already proved. So `/fix` stays
where it is; `Skill(znf:run)` still gives the repo a `dev` pane when the fix needs the app running.

**A bug found while another task is in progress is not a new task.** This is the most common way
`/fix` gets entered — mid-feature, from inside a worktree that already exists. Then the fix belongs
in that worktree: `cd` there and skip this step. `wt new` refuses a second task in the repo anyway
and prints where to go. Only a bug genuinely unrelated to the work in flight earns `--another`, and a
live production bug is `/hotfix`, which is exempt by design.

Do this **before Step 5**, not before Step 0 — Steps 0-3 only read (logs, code, git history), and
the diagnosis may well end in "this is not a code fix". Create the worktree when you are about to
edit, not when you start looking.

## Step 0: Ground in real error data

The most common debugging mistake is fixing the wrong thing. Before any analysis:

**If no argument (empty):**
```bash
# Auto-fetch recent error output
# Check: app logs, test output, terminal history, error files
ls -t *.log 2>/dev/null | head -3
tail -100 <latest-log-file> 2>/dev/null
```
Also run the project's test suite for build/test errors. **Don't assume `npm`** — use `pm`
(`pm test`, `pm run build`), which resolves the manager from the `packageManager` field then
the lockfile and refuses rather than guessing. Read the repo's CLAUDE.md first if it documents
a different verify command.

**If argument is a description:** Use it as a symptom to search for; still fetch real logs if possible.

**If argument is a GitHub Actions URL (e.g. `https://github.com/owner/repo/actions/runs/...`):**
```bash
# Fetch CI log via gh CLI
gh run view <run-id> --log 2>&1 | grep -A 20 "Error\|FAIL\|error:" | head -100
# Or: gh run view --log-failed
```

Now you have **real error data**. Proceed only with this.

## Step 1: Root cause analysis

Use superpowers `systematic-debugging`:
- What is the actual error message (exact text from logs)?
- What changed recently? (`git log --oneline -10`, `git diff HEAD~1`)

**On the Wide path: one investigator agent per hypothesis, all dispatched in ONE message.**
Identify 2-3 independent root-cause hypotheses and, for each, the specific evidence that would
confirm or refute it. Then dispatch one `Explore` agent per hypothesis — **all in a single message**,
which is what makes them run concurrently; separate messages run them one after another and buy
nothing — and **name `model: 'sonnet'` on each**. `Explore` pins no model, so an omitted one inherits
the session's; you already stated the evidence that settles each hypothesis, which makes this a search
against a fixed target rather than open-ended reasoning. Keep the reasoning at the top tier where it
belongs: forming the hypotheses (here) and confirming the survivor (Step 2), both inline. Give each one hypothesis, the evidence that would settle it, and this instruction:
*return the evidence found for AND against, plus a verdict — do not fix anything.*

This is the largest single latency in `/fix` and the reason it is worth agents rather than inline
searching: each hypothesis needs a broad sweep whose intermediate output (file lists, log greps,
match dumps) is large and worthless once the verdict is known. Investigating three hypotheses inline
lands all three sweeps in this context; three agents land three verdicts.

Kill hypotheses with evidence; don't keep ones that don't fit the actual error. **Ask each agent for
its report by name** — with several out at once, a lost report is indistinguishable from a hypothesis
that found nothing, and only one of those is safe to act on (`CLAUDE.md §3`).

Confirming the surviving hypothesis is Step 2, and it stays **inline** — that step reads real error
output, and its output is the evidence the fix is built on, not a map pointing at it.

## Step 2: Verify the real cause

Before fixing, confirm the root cause:
- Can you reproduce it? (run the failing command/test)
- Does the evidence match the hypothesis?

If you cannot confirm the root cause, state that — do not guess-fix.

## Step 3: `/scout` — what depends on the code you are about to change

The cause is confirmed; now find out what else touches it. Call **`Skill(znf:scout)`** **before**
editing, not after — after means the change is already made and you are looking for permission.

**Invoke it as a tool, not as an intention.** `Skill(znf:scout)` and the `Agent(znf:scout)` it dispatches
each leave a line carrying their name; grounding or scouting merely *performed* dissolves into a
scatter of `Bash`/`Read` calls indistinguishable from any other work. That difference decides
whether "did the scout run?" is answerable by looking or only by trusting the summary. Same for
`Skill(znf:ground)` in Step 5. **A missing line is a skipped step.**

A fix is riskier than a feature in exactly this respect: a feature adds code nothing calls yet,
while a fix changes a path that is live enough for someone to have watched it fail. The numbers
are from bug-fixing benchmarks specifically — an agent baseline broke **6.5 already-passing
tests per patch**, and impact-analysis-guided test selection cut that ~**70%** while *raising*
the resolution rate.

Emphasise the reverse direction in the brief:

1. **who calls / reads / writes** what you are changing
2. **which tests cover it** — and which changed behaviour has no test
3. **what else is written in the same operation** (queue job, cache entry, search index)
4. **why these lines exist** — `git blame -L`, `git log -S`

Target 4 carries most of the weight here, because **the bug is usually in code someone else
wrote**. You have no memory of the intent encoded in those lines, and the obvious fix may delete
exactly the case the original author was handling. A global outage was traced to a refactor that
silently removed a CPU-time guard: nobody knew why it was there.

If the report says **"cannot enumerate by grep"**, treat the map as partial and carry that
forward — do not upgrade a partial map into a clean one.

## Step 4: Choose the fix approach — then check it still belongs here

A confirmed cause usually leaves more than one way to fix it, with real trade-offs:

```
patch the symptom        vs   fix the root cause
fix in the caller        vs   fix in the callee
add a guard             vs   restructure the flow
fix forward             vs   revert the commit that introduced it
```

State the approach in one line with the alternative you rejected and why. If the choice is
genuinely obvious, say so and continue — but say it, so the decision is visible and reviewable
rather than implicit in the diff. If there is a real trade-off, put the options to the user and
wait: this is a decision about their system, not a detail of the implementation.

The chosen approach goes into the ship-pack `## Intent` block alongside the root cause, so the
reviewer can check the fix against the approach that was agreed, not just against the bug.

**Escalation door — take it rather than forcing a fix through this skill.** If the fix turns out
to span more than ~3 files, change a contract other services depend on, or require a design
decision rather than a choice between two obvious options: **stop and run `/cook`.** At that
size it is a feature-shaped change and the label "it's a bug" stops mattering — it needs a spec,
a plan, and per-task review, none of which exist in this skill. Continuing here means writing a
multi-file behavioural change inline with no independent review of the implementation.

## Step 5: Minimal fix

Make the **smallest change** that fixes the confirmed root cause, along the approach chosen in
Step 4. Do not refactor unrelated code.

If the fix touches any shape you haven't verified this session — DB field, API request/response, queue/event payload, library method signature, in-repo function/symbol you call into, env var name — call **`Skill(znf:ground)`** first, so the step leaves a line.

## Step 6: Verify the fix

Re-run whatever was failing, using the project's own verify command (not assumed to be `npm` — see Step 0). Confirm it passes, and run related tests too.

**This step and `/ship` step 4 are not the same check — know which one you are doing.** Here the
question is narrow: *did the bug go away*. `/ship` step 4 is the authoritative pass at the final
fingerprint, after any review fix, and it is the only place a UI verifier agent is dispatched.

So if the bug was visual, **do look at it in the browser here** — the main session may drive Playwright
directly, and the constraint is only against two *concurrent* browser drivers. Doing so also leaves the
browser **logged in**, which `/ship` step 4 then inherits: neither verifier agent can authenticate
itself, so that login is a prerequisite you may as well establish while confirming the fix. What you do
**not** do here is the objective layout measurement or the screenshot audit — that is the agent's job at
step 4, and duplicating it costs a second login handoff for no new information.

If there are **no tests**, "it compiles" is not verification — **`Skill(znf:run)`** to launch the app and observe the actual behaviour through the real code path, and show that output. Invoke it as a tool rather than doing its steps by hand: it reads the port `wt` allocated for this worktree instead of hunting for a free one, which is what keeps the URL you report the same as the one the app is on.

**A clean check is not yet evidence.** A positive result carries its own content: "FAIL", "error", "3 matches" tells you something on its own. A *negative* one — "no match", "0 results", "OK", "nothing found", "no diff" — that lets you **proceed** does not, until a second independent mechanism agrees. Cheap, and it applies only to negatives that open a door:

| The check said | Confirm it with |
|---|---|
| a repo-wide sweep found 0 hits | `rg`, never `grep -R`; plus one count against a file you know contains a hit |
| the config was applied | measure the effect (pixels, `getBoundingClientRect`, real output) — not by re-reading the config |
| a wrapper tool: "not found" / "not a repo" / "none" | run the underlying tool directly (`git worktree list`, not the wrapper's view of it) |
| a connection failed | `nc -z <host> <port>` first — separate network from credential before theorising about timeouts. **Run it with the sandbox disabled and say so:** inside the sandbox `nc` and `curl` report *every* port closed, so the check answers "network is down" for everything. For a local port use `lsof -nP -iTCP:<port> -sTCP:LISTEN`, which reads the kernel socket table instead of dialling and therefore works either way |

In one session, five separate checks reported clean and the clean was wrong: `grep -R` missed
a line `rg` and `grep -c` both found; `+show-config` said the setting applied while text was
clipped off-screen; a wrapper reported "not a git worktree" where two existed. No amount of
review catches this class — reviewers read the same tool output you did.

## Step 7: Report

```
Root cause: <specific cause, with the real error output that confirmed it>
Approach:   <chosen, and what was rejected>
Scout:      <blast radius; uncovered paths; confidence>
Fix:        <what changed and why>
Verified:   PASS / FAIL (at which fingerprint)
```

Then, after Step 8, **`cat` `/ship`'s board file underneath this** — the gate writes it to
`${TMPDIR:-/tmp}/ship-board-<fp10>.md` and gives you the path. Do not retype it and do not collapse it
to *"`/ship`: ✅"*: that hides every check that carries weight in this pipeline and discards the
per-check fingerprints, which are the only part the user can verify without trusting this report. A
`cat` also makes a skipped step visible as a missing tool call.

## Step 8: Gate, then ship — both, always

**Run `/gate`** (the project's contract gate): cheap, read-only, safe to run unprompted. It
tells you which repos the change reaches and fixes the deploy order. Do not decide by judgement
whether a change "looks shared" — the gate answers that.

**Then run `/ship`. Always — not only when the gate reports cross-repo impact.**

This was previously conditional on the gate, which left a single-repo fix with **no independent
review at all**: gate clean → commit → push. That is the weakest link in this skill, and the
evidence against it is specific. Agents report success when they have not succeeded: **75.8%**
of trajectories carrying a self-reported status claim were false successes, and an LLM judge
asked to detect that reaches only **AUROC 0.65** — barely better than a coin flip, so a
self-check cannot close it. What did close it was independent verification outside the agent's
own control: **48% → 3%** across comparable domains. `/ship`'s reviewer is that independent
party. A fix written and verified by the same session, with nothing else looking, is exactly the
single-control setup those numbers describe.

The cost is one agent call on a small diff. Weigh it against 6.5 broken tests per patch.

If `/ship` is all green it commits and pushes the feature branch itself (house rule #7). Never
commit on a deploy branch — the git-guard hook enforces that.

---

## Red flags

| Symptom | Action |
|---|---|
| Fix attempt #3 for same bug | Stop; re-read the actual error from logs |
| "Probably caused by X" | Not probably — confirm with evidence |
| No logs visible | Ask user to provide the actual error output |
| "It's a one-line change, skip the scout" | One line can have any number of callers. Line count is not blast radius |
| Deleting a condition/guard you don't understand | `git log -S` it first. That is how the guard that was containing an outage gets removed |
| The fix has grown to 4+ files | Take the escalation door — `/cook`. You are writing a feature with no spec and no per-task review |
| "Gate was clean, so no review needed" | Gate answers reach, not correctness. `/ship` runs regardless |
