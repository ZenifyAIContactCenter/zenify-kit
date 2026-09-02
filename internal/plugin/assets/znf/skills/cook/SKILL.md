---
name: cook
description: Full feature pipeline in one command. Use when implementing a feature end-to-end — branch, brainstorm to a spec, ground every name against real data, plan, subagent-driven implementation, then the pre-ship gate. Always spec-driven and always subagent-driven, at every size. Commits and pushes the feature branch on all-green (house rule #7); never opens the PR.
argument-hint: "<feature description or path/to/plan.md>"
allowed-tools: Read Grep Glob Bash Agent
---

**Every gate is kept, at every size.** There is no "small enough to skip" path — see below.

## What `/cook` does

```
0 fetch  → 1 ground → 2 brainstorm→spec → 3 ground → 4 scout → 5 plan → 6 wt+SDD → 7 /ship
  base       request     2 user gates       spec      what        +ground   per-task
  only                                                depends     what the  review
                                                      on it       plan adds
```

**The worktree is created at Step 6, not Step 0.** Steps 0-5 only read code and write the spec and
plan, which live in the main checkout by rule #8 — so there is nothing for a worktree to hold until
implementation starts. Step 0 is a `fetch` so that what you ground and design against is current.

**Verification sits around the two superpowers skills, never inside them.** Nothing in
`brainstorming` or `writing-plans` is reordered or overridden — the checks go before and after.

Two different questions, in two different steps, and neither substitutes for the other:

```
/ground  "what is X?"            forward — names enter at three points (request, spec, plan)
                                 and each is grounded where it enters
/scout   "what depends on X?"    outward — asked once, after the spec has decided what changes
```

Each grounding pass is incremental: `/ground` only fetches what has not been verified this
session, so a pass over which nothing new appeared costs nothing.

## Detecting input type

**Argument is a `.md` file path** → the spec/plan stage already happened. Do Step 0, then
ground **every name the file uses** (the three grounding passes collapse into one here — the
plan is already written, so there is nothing left to ground incrementally), then Step 4
(`/scout`), then Step 6. Skip Steps 1, 2 and 5.
**Argument is a description** → run every step.

## There is no complexity triage

This skill used to open with a Simple/Complex assessment that let a "Simple" feature skip
brainstorming. That is deleted, deliberately, for two reasons:

- `znf:brainstorming` forbids it in a section titled **"Anti-Pattern: 'This Is Too
  Simple To Need A Design'"**, plus a HARD-GATE that applies "to EVERY project regardless
  of perceived simplicity". The old Step 1 was the exact rationalization that skill names.
- What looks simple often isn't, and the judgement is made before the information that
  would settle it exists.

Scale the *design* to the problem — brainstorming allows "a few sentences for truly simple
projects" — but never skip a step. If the work is genuinely mechanical (nothing unknown,
one repo, no shared resource), it should not have entered `/cook` at all: it belongs on the
spine in `CLAUDE.md §0` (branch → change → verify → `/gate`).

Model choice is not decided here either — see Step 6.

## Step 0: Sync the base — fetch, and know which ref you are reading

**No worktree here.** It used to be Step 0, which was wrong: Steps 1-5 only read code and write
the spec and plan, and those go in the **main checkout** by rule #8 — so nothing needs a worktree
until Step 6. `/fix` already had this right (*"create the worktree when you are about to edit, not
when you start looking"*); `/cook` was the one out of step. What Step 0 is really for is making
sure the code you are about to ground and design against is current.

```bash
git -C <repo> fetch origin                    # per affected repo
node -e 'console.log(JSON.parse(require("fs").readFileSync(".claude/worktree.json","utf8")).baseRef)'
git -C <repo> rev-list --count HEAD..<baseRef> # how far behind the checkout is
```

**`fetch`, never `pull`, and never switch the checkout's branch.** The base is *declared*, not
guessed — `baseRef` in each repo's `.claude/worktree.json`, and it differs between repos in the
same workspace, so read it per repo and never carry one repo's answer to another. `fetch` updates
that ref whatever the checkout happens to have out, which matters because **main checkouts are
mostly not sitting on their base**: measured once across a 13-repo workspace, 8 were on someone's
feature or hotfix branch and 3 were dirty. A `pull` there either fails or advances the wrong
branch, and a `checkout` would abandon work in flight.

**Consequence for Step 1, and it is the reason this step exists:** reading a file out of the main
checkout gives you *whatever branch that checkout is on*. Ground a repo while its checkout sits on
an unrelated feature branch and you have grounded against that branch, not the base — silently, and
with every name you check coming back plausible. When the distinction matters, read the ref
explicitly:

```bash
git -C <repo> show <baseRef>:<path>
```

State, per repo, one line: declared base · is the checkout on it · commits behind · dirty. If a
repo is off-base or dirty, say so rather than reading through it silently — that is the difference
between grounding and guessing which code you grounded.

(The `.worktrees/` name and the `wt new` that creates it are in Step 6. Current `wt` fetches before
resolving the base, but Step 6 still fetches explicitly — belt-and-suspenders, and required on older
builds — because brainstorm and planning take real time and the base moves while they do.)

## Every step must leave a named line

**Invoke each sub-skill through the Skill tool — `Skill(znf:ground)`, not "go and do the grounding".**
Same for `/scout`, which is an Agent-tool dispatch and therefore always leaves `Agent(znf:scout)`.

The reason is auditability, not tidiness. A step invoked as a tool renders one line carrying its
name; a step merely *performed* dissolves into a scatter of `Bash(db_read …)` and `Read(…)` calls
that look like every other piece of work. Then "did the grounding actually happen?" is answerable
only by trusting the summary — and the whole design of this pipeline is that its steps can be
checked by looking, the way `Agent(znf:scout)` can.

So a clean `/cook` run leaves a visible spine: `Skill(znf:ground)` ×3, `Agent(znf:scout)` ×1,
`Skill(znf:brainstorming)`, `Skill(znf:writing-plans)`,
`Skill(znf:subagent-driven-development)`, `Skill(znf:ship)`. **A missing line is a skipped
step**, and that is the point.

It costs something real — invoking a skill reloads its text into context, and `/ground` runs three
times here. Pay it. An unverifiable step is worth less than the tokens it saved.

## Step 1: Ground the request — before brainstorming

Call **`Skill(znf:ground)`** on the entities the request names, before any clarifying question is asked.

This is not an override of `brainstorming` — it is doing the data half of that skill's own
step 1, "explore project context", properly, and handing the result in. The skill is then run
exactly as written.

Why before rather than after: `brainstorming` ends with **two user gates** — approval per
design section, then approval of the written spec. Grounding only after those means the user
can approve a design resting on a collection that does not exist, and the correction costs
another round of *their* review, not yours. Cheaper to arrive with the real names.

What is knowable this early is limited but it is exactly the part that keeps going wrong:

```bash
db_read collections <term-from-the-request>    # the real names, before anyone commits to one
db_read doc <a-name-from-that-list>            # the fields that actually exist
```

Enough to stop a design being built on `chatbot_setting` when the collection is
`chatbot_settings`. Field-level detail comes at Step 3, once the design says which fields it
needs.

## Step 2: Brainstorm → spec (`znf:brainstorming`)

Call **`Skill(znf:brainstorming)`**, then follow its nine steps as written. It is not a
summary step — it contains **two
user gates**, and both are real:

1. explore context → 2. visual companion, only just-in-time → 3. clarifying questions,
**one per message** → 4. two or three approaches with a recommendation, YAGNI →
5. present the design in sections, **user approves after each section** →
6. write the spec to `<main-checkout>/docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md` —
**the main checkout, not the worktree** (rule #8) →
7. spec self-review (placeholders / contradictions / scope / ambiguity) →
8. **user reviews the written spec file** → 9. hand off to `writing-plans`.

Keep the polyrepo questions in scope while doing it: which repos does this touch, which
contract boundaries, what breaks.

**When you WRITE the spec file (and the plan file at Step 5), these are project-AGNOSTIC
artifact-quality rules — a badly-formatted spec defeats its own purpose:**

Khi viết spec/plan, tuân `znf:_shared/artifact-style` (reference dùng chung: header `**Label:**
value`, không markdown table, không diagram viewer-không-render, title theo ngôn ngữ prose giữ
jargon). Xem file đó.

**Do not commit the spec, and do not `git add -f` it.** Step 6 of that skill says to commit
the design document; this project deliberately blocks it with `.gitignore` instead. Three
consequences to respect:

- Write it in the **main checkout**, never in the worktree — a worktree does not carry ignored,
  untracked files, and `wt rm` would delete it along with the branch.
- **Pass SDD absolute paths** to the spec and the plan. A relative path resolves against the
  worktree, where the file does not exist.
- `git clean -fdx` deletes every spec and plan. They are scratch, not history — so if a decision
  in there matters beyond this task, it belongs in a memory or in `CLAUDE.md`, not only here.

## Step 3: Ground the spec — before the plan, not after

Call **`Skill(znf:ground)`** again, now on everything the approved spec actually commits to. Step 1 could
only ground what the request named; the spec has since decided which fields, endpoints and
payloads are involved.

This runs **before** `writing-plans` on purpose: that skill forbids placeholders and demands
real code in every task, so a wrong collection or field name gets written *into the plan*, and
SDD then hands implementers the brief with "the exact values to use verbatim". Grounding after
the plan grounds a contaminated requirement. This is the path that put three empty junk
collections into a shared production database.

Ground all six categories, not just the DB:

- DB collections/tables and fields — **list the real names, never type one from memory**
- API endpoints and their request/response shapes
- Queue/event names and payload fields
- Library methods and their signatures (read installed types, not memory)
- In-repo code the plan calls into: signatures, exported symbols, component props, config
  keys — read the definition, not a call site
- Env var names — off the running process, not off `.env`

If grounding contradicts the spec, fix the spec first. Do not write a plan on top of it.

Ground inline. There is no DB delegate — the `db-schema-fetcher` agent was deleted after 0
dispatches in 1427 transcripts, and `/ground` explains why inline is the right place: what this
step produces is the shape the code gets written from, so a summary of it is not a substitute.

Do not proceed with any unverified name.

## Step 4: `/scout` — what depends on what the spec is about to change

Call **`Skill(znf:scout)`** once, here — it dispatches the `scout` agent, so the run leaves both
`Skill(znf:scout)` and `Agent(znf:scout)`. After the spec has decided what changes, before `writing-plans` locks
the File Structure. That decision needs to know what already exists and what depends on it.

**Dispatch it at the top of Step 3, not after Step 3 finishes.** Both steps take the approved spec as
their only input, so nothing about grounding informs the scout brief — and while `Agent(znf:scout)` sweeps,
the main loop can do Step 3's grounding inline. That overlap is free: it needs no extra agent and
changes no output, only the order the two are started in. Collect the scout report before writing the
plan, since the plan's File Structure depends on it.

The numbering stays 3 → 4 because that is the order the *results* are consumed; only the dispatch moves
earlier.

Grounding cannot answer this. It is forward-only — it tells you a name is real, never whether
changing it breaks a caller you did not know about. Opposite directions, and the second one is
where existing behaviour gets broken.

Brief for a feature is **mixed**, unlike a fix: part of the change is new code that nothing
calls yet and cannot break anything, part of it plugs into code that already has consumers.
Point the scout at the second part:

1. **who reads / writes / calls** the shared things the spec touches — in this workspace,
   delegate that to `/gate` rather than re-deriving the eight-repo sweep
2. **which tests cover** the code the plan will modify
3. **what else is written in the same operation** — a queue job, a cache entry, a search index
4. **why the existing code is the way it is**, for anything being changed rather than added

If the report says **"cannot enumerate by grep"**, carry that word "partial" into the plan.
Do not launder a partial map into a clean one.

## Step 5: Plan (`znf:writing-plans`)

Call **`Skill(znf:writing-plans)`**. Map the files first, then write tasks containing real code — no "TBD", no "add error
handling", no "similar to Task N". Run its self-review (spec coverage / placeholder scan /
type consistency). Save to `<main-checkout>/docs/superpowers/plans/<filename>.md` — **the main
checkout, same reason as the spec** (rule #8), and hand SDD the absolute path.

**Apply the same artifact-quality rules as the spec** — `znf:_shared/artifact-style`, cited under
Step 2. The plan is as much a human-read artifact as the spec.

**Decide here, once, which tasks are worth a browser run — and write it into their definition of done.**
This is the *only* thing that triggers a per-task UI check; Step 6 does not infer it from file
extensions.

The test is whether the task's **deliverable is something you look at**, not whether it happens to touch
a rendering file:

```
worth it        a new screen · a new component · a layout or grid change · a modal
                → "Done when … and `ui-verifier` reports it renders correctly, with the
                   overflow measurement of the changed element against its container."
NOT worth it    a copy change · a colour token · a css file touched in passing · wiring an
                existing component to a new endpoint
                → say nothing; `/ship` step 4 still sees it at the end
```

Getting this wrong in the cheap direction is fine — a task you did not flag is still caught by the final
gate, just without attribution to a task. Getting it wrong in the expensive direction costs a browser run
per task plus a report to chase, on the pipeline you run most.

This belongs in the plan and not only in Step 6's rules, because `scripts/task-brief` extracts each
task's text **from the plan file** with `awk`, and SDD calls that brief *"the single source of
requirements"*. A requirement written into the plan therefore arrives in front of you at dispatch time;
a requirement living in this file has to be remembered while you are reading SDD's loop instead — and a
rule in the wrong file is exactly how `/ship` came to contradict this one until a cross-check caught it.

**Then `Skill(znf:ground)` a third time, on any name the plan introduced.** Call it after the plan is
written, not inside `writing-plans` — that skill stays untouched.

It has no grounding step of its own. Verified: its self-review checks the plan against the
spec, and against itself ("match what you defined in **earlier tasks**") — never against
reality. So a plan can be fully self-consistent, cover every spec requirement, carry no
placeholder, and have every field name wrong. `implementer-prompt.md` does not check either;
its only verification lines are about tests. Step 3 grounded what the *spec* committed to;
this covers what the *plan* added — and plans do add names, because this skill demands real
code in every task. Without this pass the only net left is the ship-pack's `## Ground` block
at `/ship`, which fires after the code is already written.

That skill ends by offering an execution choice. **The answer is already fixed: always
Subagent-Driven. Do not ask.** `executing-plans` is not used here — it defers to SDD itself
when subagents are available, and it has no code review of any kind: no per-task reviewer,
no fix loop, no final review.

## Step 6: Implement (`znf:subagent-driven-development`)

### First, the worktree — this is the step that writes code

**A worktree, always — house rule #8, no conditions.** It belongs *here*, not at Step 0: the
plan is agreed, so this is the moment the first line of repo code gets written. Nothing inspects
`git status --porcelain` to decide, because there is no decision.

```bash
git -C <repo> fetch origin                                    # belt-and-suspenders — current wt fetches too, older builds do not
cd <repo> && wt new <slug> --type feat --base "$(node -e 'console.log(JSON.parse(require("fs").readFileSync(".claude/worktree.json","utf8")).baseRef)')"
```

**Fetch again even though Step 0 fetched.** The base moved while brainstorming and planning
happened — that gap is exactly what the reordering introduced. Current `wt` fetches before resolving
the base, but keep this explicit fetch: it is required on older builds and harmless on new ones. Skip
it on an older build and you get a worktree that looks freshly based and is not, discovered at merge
time.

**Polyrepo:** one worktree per affected repo, **same slug** in every one.

**Re-entering `/cook` mid-task does not mean a second worktree.** If this feature already has one,
`cd` into it — `wt new` will refuse and print the path (house rule #8). The slug belongs to the whole
**plan**, fixed at the first `wt new`; it does not get re-derived from whatever the latest message was
about, and it does not subdivide — every `Task 1..N` below shares this one worktree per repo.

`znf:subagent-driven-development` Setup will otherwise create a worktree of its own and
assumes a single repo. Tell it the workspace already exists so it only verifies.

**Spec and plan stay in the MAIN checkout, and are already written by now.** They are gitignored, so
they do not follow a worktree, and `wt rm` would delete them. The worktree holds code only. Every
path handed to SDD must therefore be **absolute** — a relative path resolves inside the worktree
and finds nothing.

### Then hand the slug its own workspace — once, and only from outside it

Steps 0-5 belong in the workspace you started in: it holds the spec, the plan, and `/gate`. Step 6
onward belongs somewhere that is *this slug and nothing else*, with the repo's dev servers beside it.
When herdr is running, move the work there. Skip this whole section when `$HERDR_WORKSPACE_ID` is
unset — everything below is an optimisation of where work happens, never a precondition for it.

**The loop guard comes first, because getting it wrong breaks this in one of two directions.** The
session that receives the handoff re-enters `/cook` with a plan file and arrives back here; it must
not hand off again. The condition is **"am I in the workspace bound to this worktree?"**, which only
`herdr worktree open` ever sets:

```bash
HERE=$(herdr workspace get "$HERDR_WORKSPACE_ID" \
        | node -pe 'const w=JSON.parse(require("fs").readFileSync(0)).result.workspace; w.worktree ? w.worktree.checkout_path : ""')
[ "$HERE" = "$(cd <repo> && wt path <slug>)" ] && echo SKIP      # already home → straight to SDD
```

**The obvious guard is wrong, and it failed in exactly this way on the first real run.** It was
`git config --get wt.slug` — non-empty meaning "already inside the worktree". But `wt.slug` is
worktree-scoped git config, and the block immediately above this one tells you to `cd` into the
worktree and work there. So by the time this guard is read, cwd is inside the worktree, `wt.slug` is
set, and **the handoff is skipped by the very session that just created the worktree** — deterministically, not by luck. Observed: 11 worktrees in `contact-center-hub`, each with `wt.slug`
set, a session working inside one of them, and no per-slug workspace ever opened.

A workspace created by `workspace create` carries **no** `worktree` field, so this test also
distinguishes "a workspace whose cwd happens to be the repo" from "the workspace of this slug".
`$HERDR_WORKSPACE_ID` unset → no herdr → skip the section entirely, as below.

```bash
ORIG="$HERDR_WORKSPACE_ID"                              # where the spec and plan live
W=$(herdr worktree open --cwd "<repo>" --path "$(wt path <slug>)" \
      --label "<slug>" --no-focus --json | node -pe 'JSON.parse(require("fs").readFileSync(0)).result.workspace.workspace_id')
P=$(herdr pane list --workspace "$W" --json | node -pe '...root pane id...')
herdr agent start "<slug>" --kind claude --pane "$P"
herdr workspace focus "$ORIG"                           # unconditionally — see below
herdr agent prompt "<slug>" "/cook <ABSOLUTE path to the plan file>"
```

Four things that each cost a wrong turn earlier:

- **`--no-focus`, then `workspace focus "$ORIG"` anyway.** `agent start` has no `--no-focus`, and
  whether it steals focus measured *differently on two runs*. Restoring unconditionally is correct
  either way and costs one call. Not focusing is the right default regardless: with several repos in
  flight, any automatic choice of where to look is wrong, and only the user knows which repo matters.
- **The plan path must be absolute.** The new session's cwd is the worktree; a relative path resolves
  there and the plan does not exist there — rule #8 keeps it in the main checkout.
- **`agent prompt` after `focus`,** so the prompt lands in a session that is already settled.
- **This is a handoff, not a migration.** The running conversation cannot move — `claude --resume`
  needs the old session dead first, and the session issuing these commands is the one that would have
  to die. What crosses over is the plan file, which `writing-plans` already requires to be
  self-contained. If the new session cannot work from it, the plan was incomplete, and finding that
  out is worth more than the convenience it costs.

Then **stop**. Report the workspace, the pane and the branch, and let the handed-off session run
Steps 6-7. Do not also run SDD here: two sessions on one worktree is the contaminated-review case
this skill spends a whole section forbidding.

### Start the app once, if anything in the plan has to be exercised

**`Skill(znf:run)` here, not only at the UI check.** Until this was added, `/run` was reachable from
exactly two places — the per-task `ui-verifier` check, and `/ship` step 4, which prefers tests when
they exist. A backend task therefore reached neither: hub has 81 MCP specs, so step 4 runs those and
never starts anything, and there is no UI to verify. The observed result was a `/cook` run with no dev
server anywhere, which is also why nothing appeared in a `dev` pane.

The trigger is the plan, same as everything else here: **if any task's definition of done can only be
shown by calling the running app** — an endpoint, a tool a client invokes, a queue consumer, a socket
event — invoke `Skill(znf:run)` once, before the task loop, and keep it up for the whole run. Rule #3 is
what makes this compulsory rather than convenient: a change whose new behaviour no test covers is
unverified until real output from the real code path exists, and a passing suite that never touches
the new path is not that output.

Nothing to exercise — a refactor fully covered by tests, a docs change — then skip it and say so.

### Then SDD

Call **`Skill(znf:subagent-driven-development)`**. Always SDD, at every size. Per task: brief → implementer (code + test + commit + self
review) → task reviewer (spec compliance **and** quality) → fix loop, capped at five
rounds with a scoped re-review each round → ledger line. Then one final whole-branch
review on the most capable model.

- Tell SDD the workspace created just above already exists; it should verify, not create.
- **Implementers: `model: 'sonnet'` with `effort: 'xhigh'`, every task.** State both on every
  dispatch — SDD requires the model to be explicit, and an omitted `effort` inherits the
  session's.
- **This overrides SDD's cheapest tier deliberately — do not "fix" it back.** SDD says a task
  whose brief contains the complete code is transcription and takes the cheapest tier. That is
  right about the reasoning needed and wrong about the *dial*: the cheapest tier is
  `claude-haiku-4-5`, and haiku 4.5 is **not xhigh-capable** — it appears in the CLI's
  `xhigh_effort` exclusion list alongside `claude-3-*`, `opus-4-0/4-1/4-5/4-6` and
  `sonnet-4-0/4-5/4-6`, while `sonnet-5`, `opus-4-7/4-8`, `opus-5` and `fable-5` do not. So
  dispatching haiku at `xhigh` does not raise effort and does not fail either — the CLI
  **silently downgrades** it (`"Effort '<x>' exceeds … using '<y>'"`), so the dispatch looks
  correct and the effort is gone.
  Since effort matters more than tier for coding, dropping to haiku trades away the stronger
  dial to save on the weaker one. SDD's own *"turn count beats token price"* points the same
  way: the cheapest tier takes 2-3× the turns on multi-step work and costs more overall.
  **Sonnet is therefore the floor, not a starting point to scale down from.**
- Scale **up** to `opus-4-8` only for a task needing design judgment or broad codebase
  understanding — and reach it by **omitting** `model`, never by passing `'opus'`: that alias
  resolves to the newest opus and would override the version pinned in `~/.claude/settings.json`.
- Minimum code to satisfy the plan's definition of done. TDD: failing test → implement →
  pass. Match existing style; no upgrades to unrelated code.

### Parallel implementers: across repos yes, within one repo no

SDD says *"Never dispatch multiple implementation subagents in parallel (conflicts)"* (`SKILL.md:230`).
Read alongside `SKILL.md:238` that prohibition is narrower than it sounds, and the real boundary is
sharper:

```
scripts/review-package PLAN_FILE BASE HEAD
  BASE = the commit recorded BEFORE dispatching this implementer
```

Two implementers sharing **one git history** means `HEAD` holds both sets of commits, so task A's
review package contains task B's diff and the reviewer grades A against code it never wrote. That
fails **silently** — no error, a plausible review of the wrong thing. So:

```
different repos (be / web / hub / subscriber)   separate histories already   → PARALLEL
same repo, same worktree                        review package contaminated  → NEVER
same repo, one worktree per implementer         safe, but N branches to merge → OFF by default
```

**Dispatch the cross-repo group in ONE message** — that is what makes them concurrent; one message
each runs them in sequence and buys nothing.

**Why cross-repo needs no coordination:** the web implementer does not wait on the backend
implementer, because the shape of the new endpoint is in the **plan**, not in the backend
implementer's output. `writing-plans`' rule that a plan contain real code and no placeholders —
written for an unrelated reason — is exactly what makes this safe. If a task cannot start without
another task's *output*, the plan is under-specified; fix the plan, don't serialise around it.

`/ship` step 6's `schema → backend → subscriber → frontend` is **deploy** order, not implement order.
It constrains nothing here.

**Same-repo parallel is off by default and that is a decision, not an omission.** Within one repo
tasks are usually genuinely ordered (schema → service → controller), so parallelising is machinery
built to run a sequence. It is also the harder case, not the easier one: several services inside one
repo — `hub`'s `apps/*` — have disjoint files but **one history**, so they need a worktree per
implementer, and the merge that follows is where the conflict returns, just later. Turn it on only
when the plan itself states two tasks touch disjoint files with no ordering dependency.

**With several implementers out at once, ask each for its report by name.** A lost report and a task
that finished quietly are indistinguishable from here, and only one is safe to build on
(`CLAUDE.md §3`). Do not write a ledger line for a task whose report never arrived.

**If the plan has tightly-coupled tasks, that is a plan defect — go back and re-decompose.**
SDD routes coupled tasks away from itself, but the answer is to fix the decomposition, not
to switch executor.

### The ledger's "review clean" does not cover appearance

The receipt for this step is SDD's ledger — `<repo-root>/.znf/sdd/<plan>/progress.md`, one
`Task <N>: complete (commits a1b2c3d..d4e5f6a, review clean)` per task. Read it, but know exactly what
that phrase does and does not certify.

**It certifies nothing visual.** Verified by reading the prompts: `task-reviewer-prompt.md` has zero
mentions of screenshot, visual, browser, render or look, and `implementer-prompt.md` has no browser tool
at all. So the task reviewer grades "Spec Compliance" by reading a diff — and for a task whose
deliverable *is* a layout, it is grading blind. A UI task can be written into the ledger as
`review clean` with nobody having looked at it.

Consequence for how you read the receipt: `review clean` means *the diff matched the brief and the code
is sound*, and nothing more.

**So label it honestly.** A task with no visual verdict gets
`Task <N>: complete (commits …, review clean — appearance not checked)`. That costs nothing and it is the
right fix for a receipt that overclaims: correct the claim, do not manufacture work to make the claim
true. A visual verdict is worth a browser run when the task's deliverable is something you look at — not
merely as a way to justify the word "clean".

### A task the plan flagged gets looked at before its ledger line is written

**The trigger is the plan, and only the plan.** If the task's definition of done asks for a `ui-verifier`
verdict (Step 5 decided that), then after the task reviewer passes and **before** appending
`Task <N>: complete`, dispatch `ui-verifier` scoped to **that task's deliverable only** — the screen or
component this task produced, not the whole feature. Its verdict joins the ledger line.

**`Skill(znf:run)` first, and take the URL from it.** `ui-verifier` is project-agnostic: it drives whatever
URL the caller hands it, so a wrong port produces a failure indistinguishable from a broken change.
`/run` reads the port this worktree was allocated instead of hunting for a free one, starts the server
in a pane beside the agent, which keeps its full height — reusing whatever is already serving that
port rather than starting a second copy — and reports the URL. Invoke it as a tool, not as an intention — the same
auditability rule as every other step here: a `Skill(znf:run)` line is checkable, "I started the app" is not.
Started once, it stays up for the rest of the run; later tasks reuse it rather than restarting.

No such line in the brief → no browser run. Do not re-derive the decision here from file extensions: an
earlier version of this section did, and it fired on any task that so much as touched a `.css` file,
which is a browser run bought with nothing.

**This check does not parallelise, even when the implementers around it do.** The Playwright browser is
a single shared instance, so if two repos' tasks are running concurrently and both are flagged, their
verifier runs go **one after the other** — and the main session must not touch Playwright while either
is running. Implementers are concurrent; the browser is a serial resource inside that concurrency. A
task whose verifier has not run yet does not get its ledger line, so a queued browser run holds up
exactly one task rather than the whole group.

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

**Serialisation is not a problem, and the reason is in SDD itself:** `SKILL.md:230` —
*"Never dispatch multiple implementation subagents in parallel (conflicts)."* Tasks run one at a time, so
only one agent can ever be holding the single shared browser. The rule against two browser drivers is
about concurrency, and there is none here.

None of this edits the plugin. It is a constraint on the brief you hand SDD and on when you write the
ledger line, both of which are yours.

## Step 7: Pre-ship gate

Run `/ship`: lint + build → the project's contract gate → behavioural verification → the contract review
lens → deploy order → commit + push the feature branch. It does **not** open the PR.

There is no separate review step before this. SDD's final whole-branch review already
covered generic quality; `/ship`'s reviewer covers what that one does not — cross-service
contracts, unverified field names, N+1. Running `code-reviewer` here as well reviewed the
same diff a third time with the same rubric as `/ship`.

**`cat` `/ship`'s board file — do not retype or summarise it.** The gate writes it to
`${TMPDIR:-/tmp}/ship-board-<fp10>.md` and hands you the path; end this step by running
`cat <that-path>`. Summarising it as *"`/ship`: ✅ all green"* hides every check that carries weight in
this pipeline and throws away the per-check fingerprints — the only part of this run the user can verify
without trusting my summary. Reading it out of a file rather than composing it also means a skipped step
leaves a **visible hole where a tool call should be**, instead of a sentence that reads fine either way.

That is also the answer to why these checks live in a skill rather than as steps 8-13 here: **one copy
of the logic, three copies of the output.** Copying the checks into `/cook`, `/fix` and `/hotfix` would
let three versions drift apart silently — which has already happened twice in this toolkit between this
file and `/ship`, both times caught only by a cross-file grep. A reprinted board cannot drift, because
it is generated fresh by the gate on every run.

---

## Which model runs which step

Steps 0 through 5 run in the **main loop** on the session model — keep the session on Opus
for `/cook`. Brainstorming cannot be delegated: it needs back-and-forth with the user.

| Step | Runs as | Model | Effort |
|---|---|---|---|
| 0 Fetch the base | main loop | session | session |
| 6 Workspace handoff | **a new Claude session** in the task's pane | inherits `settings.json` — measured `Opus 4.8`, so pass no override | session default |
| 1 Ground the request | main loop (skill) | session | session |
| 2 Brainstorm → spec | main loop (skill) | session — **keep on Opus** | session |
| 3 Ground the spec | main loop (skill) | session — all six categories | session |
| 4 Scout | **`scout` agent** | sonnet (pinned in the agent definition) | default |
| 5 Plan (+ ground what it adds) | main loop (skill) | session — **keep on Opus** | session |
| 6 Implement | subagents via SDD | **`sonnet` (floor — never haiku)**; omit `model` for a design-judgment task | **`xhigh`** |
| 6 Fix loop r1-3 | resume the same implementer | unchanged | **`xhigh`** |
| 6 Fix loop r4-5 | fresh implementer, +1 tier | omit `model` → `opus-4-8` | **`xhigh`** |
| 6 Task review | subagents via SDD | `sonnet`, or omit for a high-risk diff (SDD's rule) | default |
| 6 Final review | subagent via SDD | omit `model` → `opus-4-8` (the ceiling) | default |
| 6 UI check, flagged tasks | `ui-verifier` agent | sonnet (pinned) | default |
| 7 Ship review | `code-reviewer` agent | **pass `model` explicitly, scaled to the diff** — see `/ship` step 5 | default (`high`) |
| 7 Ship UI check | `ui-verifier` agent | sonnet (pinned) | default |

Two steps are delegated to agents for the same reason, and it is not cost: their work produces a
large volume of intermediate output — search hits, file lists, diffs — and that volume landing in
the main loop's context measurably degrades it. Routing search through a dedicated agent that
returns short `file:line` lists cut main-context tokens ~60% and *raised* accuracy. Step 4
(`scout`) and Step 7's reviewer both hand back a short report instead.

All three grounding steps run inline, however heavy the fetch. `/ground` gives the reason and the
history: the delegate that used to be offered here was never once used in 1427 transcripts, and
inline was the correct place anyway.

**The ship reviewer is scaled, not pinned.** Its agent definition says `opus`, and that used to be taken
as the answer for every review — which meant a one-line typo fix got a top-tier review on a gate that runs
after every `/fix`. SDD's own rule is the correct one and it is stated plainly there: *"Review tasks:
choose the model … scaled to the diff's size, complexity, and risk. A small mechanical diff does not need
the most capable model."* So `/ship` passes `model` explicitly **only to scale down** — sonnet for a small
diff and for the scoped re-review — and **omits it** for anything large or touching a shared contract, auth,
or tenant scoping, so that dispatch inherits the session model. Do not pass `'opus'` (Step 6 gives the
reason — the alias resolves to the newest opus, overriding the pinned version). Never below sonnet:
*"turn count beats token price"*, and the cheapest tier takes 2-3× the turns while reviewing worse.

**Floor and ceiling.** The floor is **sonnet**, not the cheapest tier — see Step 6 for why (haiku 4.5
is not xhigh-capable, so it silently discards the dial that matters most for coding). The ceiling is
`opus-4-8`, for architecture, for a task needing broad codebase understanding, and for the final
whole-branch review.

**Naming is asymmetric:** scale *down* by passing `model: 'sonnet'`; scale *up* by **omitting `model`**
so the dispatch inherits the session (Step 6 gives the mechanism — the `opus` alias resolves to the
*newest* opus and would override the pinned version). SDD's *"always specify the model explicitly"* assumed
a session default that is the most expensive model; here the session default **is** the intended ceiling,
so omission is the correct way to reach it rather than an oversight. State in the dispatch note which one
you meant, so an omission is never read as forgetting.

## Constraints preserved from house rules

- No commit before the gate passes — verify first, then commit (rule #3)
- No PR creation, no merge (rule #7)
- No push to deploy branches (rule #7)
- Correctness-first: no `--fast`/minimal-planning mode
