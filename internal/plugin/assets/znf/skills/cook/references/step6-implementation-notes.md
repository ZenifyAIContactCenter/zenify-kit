<!-- Moved verbatim from cook/SKILL.md § Start the app once, § Then SDD, § Parallel implementers,
§ The ledger's "review clean" does not cover appearance, § A task the plan flagged, § Step 7:
Pre-ship gate, § Which model runs which step (W4 slim-skills). Read when: you want to change a
model tier, run implementers in parallel, or skip the per-task UI look. -->

**Contents:** starting the app · SDD dispatch · parallel implementers (cross-repo yes, same-repo no)
· what "review clean" does not certify · the flagged-task UI check · the pre-ship gate · which model
runs which step · model rows dropped from the body table.


### From § Start the app once — the rule #3 tie-in (tier two)

Rule #3 is what makes this compulsory rather than convenient: a change whose new behaviour no test
covers is unverified until real output from the real code path exists, and a passing suite that
never touches the new path is not that output.

### From § Start the app once, if anything in the plan has to be exercised

**`Skill(znf:run)` here, not only at the UI check.** Until this was added, `/run` was reachable from
exactly two places — the per-task `znf:ui-verifier` check, and `/ship` step 4, which prefers tests when
they exist. A backend task therefore reached neither: hub has 81 MCP specs, so step 4 runs those and
never starts anything, and there is no UI to verify. The observed result was a `/cook` run with no dev
server anywhere, which is also why nothing appeared in a `dev` pane.

### From § Then SDD — why state both explicitly (extra, tier two)

SDD requires the model to be explicit, and an omitted `effort` inherits the session's.

### From § Then SDD

**Defer to SDD's Model Selection — do not re-derive or floor it here.** SDD already routes the
implementer: least powerful model that can handle the task, cheapest tier for a transcription task
(the plan carries the code), standard from prose / integration, most capable for design judgment,
specified explicitly on every dispatch. That is superpowers' text, and it is right — the
dispatcher judges the tier per task. An earlier version of this file overrode it with a hard
`sonnet` + `xhigh` floor; that was a mistake and is removed.

**The one local caveat SDD cannot know — the CLI's `xhigh` exclusion.** `claude-haiku-4-5` is
**not xhigh-capable**: it is on the CLI's `xhigh_effort` exclusion list alongside `claude-3-*`,
`opus-4-0/4-1/4-5/4-6` and `sonnet-4-0/4-5/4-6`, while `sonnet-5`, `opus-4-7/4-8`, `opus-5` and
the top tier above it are not. So dispatching haiku *at `xhigh`* neither raises effort nor errors — the CLI
**silently downgrades** it (`"Effort '<x>' exceeds … using '<y>'"`). So **never pair the cheapest
tier with `xhigh`**: a transcription task runs haiku at its default effort (which is all it needs),
and `xhigh` goes to `sonnet`+ where the dial earns its cost. This is a caveat on *effort*, not a
reason to raise the *tier*.

SDD's *"turn count beats token price"* still applies: a cheap tier can take 2-3× the turns on
multi-step work. Transcription is low-step by construction, so it rarely bites; when a cheap
implementer does keep failing its tests, the fix loop's round-4 escalation (+1 tier) catches it —
do not over-power every task in advance to avoid a case the loop already handles.

### From § Parallel implementers: across repos yes, within one repo no (tier two)

SDD's rule "Never dispatch multiple implementation subagents in parallel" is narrower than it
sounds, and the real boundary is sharper:

```
scripts/review-package PLAN_FILE BASE HEAD
  BASE = the commit recorded BEFORE dispatching this implementer
```

Two implementers sharing **one git history** means `HEAD` holds both sets of commits, so task A's
review package contains task B's diff and the reviewer grades A against code it never wrote. That
fails **silently** — no error, a plausible review of the wrong thing.

`/ship` step 6's `schema → backend → subscriber → frontend` is **deploy** order, not implement order.
It constrains nothing here.

**Dispatch the cross-repo group in ONE message** — that is what makes them concurrent; one message
each runs them in sequence and buys nothing. **With several implementers out at once, ask each for
its report by name**: a lost report and a task that finished quietly are indistinguishable from
here, and only one is safe to build on. Never write a ledger line for a task whose report never
arrived. **Tightly-coupled tasks are a plan defect** — go back and re-decompose rather than switch
executor.

### From § Parallel implementers: across repos yes, within one repo no

**Why cross-repo needs no coordination:** the web implementer does not wait on the backend
implementer, because the shape of the new endpoint is in the **plan**, not in the backend
implementer's output. `writing-plans`' rule that a plan contain real code and no placeholders —
written for an unrelated reason — is exactly what makes this safe. If a task cannot start without
another task's *output*, the plan is under-specified; fix the plan, don't serialise around it.

**Same-repo parallel is off by default and that is a decision, not an omission.** Within one repo
tasks are usually genuinely ordered (schema → service → controller), so parallelising is machinery
built to run a sequence. It is also the harder case, not the easier one: several services inside one
repo — `hub`'s `apps/*` — have disjoint files but **one history**, so they need a worktree per
implementer, and the merge that follows is where the conflict returns, just later. Turn it on only
when the plan itself states two tasks touch disjoint files with no ordering dependency.

### From § The ledger's "review clean" — why the label, not manufactured work (extra, tier two)

That costs nothing and it is the right fix for a receipt that overclaims: correct the claim, do
not manufacture work to make the claim true.

### From § The ledger's "review clean" — the visual-verdict tail (tier two)

A visual verdict is worth a browser run when the task's deliverable is something you look at — not
merely as a way to justify the word "clean".

### From § The ledger's "review clean" does not cover appearance

**It certifies nothing visual.** Verified by reading the prompts: `task-reviewer-prompt.md` has zero
mentions of screenshot, visual, browser, render or look, and `implementer-prompt.md` has no browser tool
at all. So the task reviewer grades "Spec Compliance" by reading a diff — and for a task whose
deliverable *is* a layout, it is grading blind. A UI task can be written into the ledger as
`review clean` with nobody having looked at it.

Consequence for how you read the receipt: `review clean` means *the diff matched the brief and the code
is sound*, and nothing more.

### From § A task the plan flagged — the server persists across tasks (extra, tier two)

Started once, it stays up for the rest of the run; later tasks reuse it rather than restarting.

### From § A task the plan flagged — why the exact URL and why a tool call (extra, tier two)

`znf:ui-verifier` is project-agnostic: it drives whatever URL the caller hands it, so a wrong port
produces a failure indistinguishable from a broken change. `/run` starts the server in a pane
beside the agent, which keeps its full height — reusing whatever is already serving that port
rather than starting a second copy. Invoke it as a tool, not as an intention — the same
auditability rule as every other step here: a `Skill(znf:run)` line is checkable, "I started the
app" is not.

### From § A task the plan flagged gets looked at before its ledger line is written (tier two)

Do not re-derive the decision here from file extensions: an earlier version of this section did,
and it fired on any task that so much as touched a `.css` file, which is a browser run bought with
nothing.

**This check does not parallelise, even when the implementers around it do.** The browser is a
single shared instance, so two flagged tasks' verifier runs go one after the other, and the main
session must not touch it while either is running. Implementers are concurrent; the browser is a serial resource inside that concurrency. A
task whose verifier has not run yet does not get its ledger line, so a queued browser run holds up
exactly one task rather than the whole group.

### From § A task the plan flagged gets looked at before its ledger line is written

**If a flagged task's result cannot be viewed on its own, the plan is wrong, not the check.**
`writing-plans` already requires that *"each task ends with an independently testable deliverable"* — a
UI task you cannot look at in isolation (component added in task 2, wired to the API in task 4) violates
that rule. Re-decompose, exactly as for tightly-coupled tasks.

**`/ship` step 4 still runs, and it is deliberately broader than this one.** That gate triggers
mechanically on the diff's file extensions, so it catches rendering changes this step never flagged — and
it has to, because `/fix` and `/hotfix` reach it with no plan at all. Narrow and targeted early, cheap and
broad at the end. Neither replaces the other: a per-task check cannot see a layout broken three tasks
later, and a final check cannot tell you which task broke it.

**The cost, stated plainly:** one browser dispatch per *flagged* task, and each inherits the measured
3-in-5 rate at which an agent finishes without its report arriving (`CLAUDE.md §3` — ask for it; silence
is not a clean look). Worth paying wherever a frontend is a primary focus: rendering tasks are then
the common case rather than the exception, so the cost and the benefit both land on the work you
actually do.

**Serialisation is not a problem, and the reason is in SDD itself:** SDD's rule "Never dispatch
multiple implementation subagents in parallel" means tasks run one at a time, so only one agent can
ever be holding the single shared browser. The rule against two browser drivers is about
concurrency, and there is none here.

None of this edits the plugin. It is a constraint on the brief you hand SDD and on when you write the
ledger line, both of which are yours.

### From § Step 7: Pre-ship gate — no separate review, and the board file (tier two)

There is no separate review step before this, and under `/cook` SDD skips its final review too:
`/ship`'s engine review is the one whole-branch pass. Before 2026-09-18 every diff met three
reviewers (task, SDD final, ship) — 76 ship-gate runs blocked 3 times and the one SDD ledger with
a final review recorded zero review-driven fixes, so the middle layer cost without catching.

Summarising the board as *"`/ship`: ✅ all green"* hides every check that carries weight in
this pipeline and throws away the per-check fingerprints — the only part of this run the user can verify
without trusting my summary. Reading it out of a file rather than composing it also means a skipped step
leaves a **visible hole where a tool call should be**, instead of a sentence that reads fine either way.

### From § Step 7: Pre-ship gate

That is also the answer to why these checks live in a skill rather than as steps 8-13 here: **one copy
of the logic, three copies of the output.** Copying the checks into `/cook`, `/fix` and `/hotfix` would
let three versions drift apart silently — which has already happened twice in this toolkit between this
file and `/ship`, both times caught only by a cross-file grep. A reprinted board cannot drift, because
it is generated fresh by the gate on every run.

### From § Floor and ceiling / Naming is asymmetric

The floor is **whatever tier SDD's Model Selection lands on** — the cheapest tier for a
transcription task, a standard tier from prose / integration — not a fixed model pinned here. The
ceiling is the session model (`opus-5-5[1m]`), for architecture, for a task needing broad codebase
understanding, and for the final whole-branch review. Full-ID pinning is the **session's** job
(`~/.claude/settings.json`); a subagent dispatch names a *tier* the dispatcher judged, so this file
does not hard-pin one. The only fixed rule is the effort caveat above: never the cheapest tier with
`xhigh`.

Since 2026-09-18 the ceiling is reached by passing `model: 'opus'`, not by leaving `model` out:
`zenify up` writes `CLAUDE_CODE_SUBAGENT_MODEL=sonnet` into `~/.claude/settings.json`, so a dispatch
without `model` lands on sonnet. Measured before the change, 19% of dispatches left `model` out and
inherited the Opus session — the "omission means ceiling" doctrine was the cause. `'opus'` is an
alias for the newest opus, which may differ from the session's pinned version; that gap is accepted
for subagents.

### From § Which model runs which step

Two steps are delegated to agents for the same reason, and it is not cost: their work produces a
large volume of intermediate output — search hits, file lists, diffs — and that volume landing in
the main loop's context measurably degrades it. Routing search through a dedicated agent that
returns short `file:line` lists cut main-context tokens ~60% and *raised* accuracy. Step 4
(`scout`) and Step 7's reviewer both hand back a short report instead.

All three grounding steps run inline, however heavy the fetch. `/ground` gives the reason and the
history: the delegate that used to be offered here was never once used in 1427 transcripts, and
inline was the correct place anyway.

**The ship reviewer is scaled by the engine, not pinned.** `code-reviewer`'s frontmatter says `sonnet`
(the floor); `znf:review` passes `opus` explicitly where its tier rule demands it — T3, and the
`contracts` dimension when the diff touches a shared resource. Never below sonnet: *"turn count beats
token price"*, and the cheapest tier takes 2-3× the turns while reviewing worse.

### Model rows dropped from the body table (token diet 2026-09-18)

| Step | Runs as | Model | Effort |
|---|---|---|---|
| 6 Workspace handoff | a new session in the task's pane | session (`settings.json`) | session default |
| 6 Task review | subagents via SDD | `sonnet`, `opus` for a high-risk diff (SDD's rule) | default |

Implementers follow SDD's Model Selection: the least powerful model that can handle each task,
named explicitly on every dispatch — cheapest tier for a transcription task where the plan carries
the code, standard from prose or integration work, `opus` for design judgment. The dispatcher
judges per task; nothing is hard-pinned at this step. Never pair the cheapest tier (`haiku`) with
`effort: 'xhigh'`: haiku is not xhigh-capable and the CLI silently downgrades it, so `xhigh` goes
to `sonnet` and above only.
