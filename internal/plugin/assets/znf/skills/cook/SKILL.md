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

**The worktree is created at Step 6, not Step 0** (`references/base-ref-archaeology.md` has why).

**Verification sits around the two superpowers skills, never inside them.** Nothing in
`brainstorming` or `writing-plans` is reordered or overridden — the checks go before and after.

Two different questions, in two different steps, and neither substitutes for the other:

```
/ground  "what is X?"            forward — names enter at three points (request, spec, plan)
                                 and each is grounded where it enters
/scout   "what depends on X?"    outward — asked once, after the spec has decided what changes
```

> Why: see `references/grounding-and-scout-rationale.md` — why grounding is incremental.

## Detecting input type

**Argument is a `.md` file path** → the spec/plan stage already happened. Do Step 0, then
ground **every name the file uses** (the three grounding passes collapse into one here — the
plan is already written, so there is nothing left to ground incrementally), then Step 4
(`/scout`), then Step 6. Skip Steps 1, 2 and 5.
**Argument is a description** → run every step.

## There is no complexity triage

Scale the *design* to the problem — brainstorming allows "a few sentences for truly simple
projects" — but never skip a step. If the work is genuinely mechanical (nothing unknown,
one repo, no shared resource), it should not have entered `/cook` at all: it belongs on the
spine in `CLAUDE.md §0` (branch → change → verify → `/gate`).

Model choice is not decided here either — see Step 6.

## Step 0: Sync the base — fetch, and know which ref you are reading

```bash
git -C <repo> fetch origin                    # per affected repo
node -e 'console.log(JSON.parse(require("fs").readFileSync(".claude/worktree.json","utf8")).baseRef)'
git -C <repo> rev-list --count HEAD..<baseRef> # how far behind the checkout is
```

**`fetch`, never `pull`, and never switch the checkout's branch.** The base is `baseRef` in each
repo's `.claude/worktree.json` — read it per repo, never carry one repo's answer to another.

**When the distinction matters for Step 1, read the ref explicitly:**

```bash
git -C <repo> show <baseRef>:<path>
```

State, per repo, one line: declared base · is the checkout on it · commits behind · dirty. If a
repo is off-base or dirty, say so rather than reading through it silently.

> Why: see `references/base-ref-archaeology.md` — why Step 0 moved out, drift measurement, why
> this matters for Step 1, where `.worktrees/` fits.

## Every step must leave a named line

**Invoke each sub-skill through the Skill tool — `Skill(znf:ground)`, not "go and do the grounding".**
Same for `/scout`, which is an Agent-tool dispatch and therefore always leaves `Agent(znf:scout)`.

So a clean `/cook` run leaves a visible spine: `Skill(znf:ground)` ×3, `Agent(znf:scout)` ×1,
`Skill(znf:brainstorming)`, `Skill(znf:writing-plans)`,
`Skill(znf:subagent-driven-development)`, `Skill(znf:ship)`. **A missing line is a skipped
step**, and that is the point.

> Why: see `references/why-no-triage-and-named-lines.md` — why the old triage was deleted,
> auditability not tidiness, cost of invoking a skill.

## Step 1: Ground the request — before brainstorming

Call **`Skill(znf:ground)`** on the entities the request names, before any clarifying question is asked.

What is knowable this early is limited, but it is where things go wrong:

```bash
zenify db-read collections <term-from-the-request>    # the real names, before anyone commits to one
zenify db-read doc <a-name-from-that-list>            # the fields that actually exist
```

Field-level detail comes at Step 3, once the design says which fields it needs.

## Step 2: Brainstorm → spec (`znf:brainstorming`)

Call **`Skill(znf:brainstorming)`**, then follow its nine steps as written. It is not a
summary step — it contains **two
user gates**, and both are real.

Keep polyrepo questions in scope: which repos this touches, which contract boundaries, what breaks.

**When you WRITE the spec file (and the plan file at Step 5), these are project-AGNOSTIC
artifact-quality rules — a badly-formatted spec defeats its own purpose:** follow
`znf:_shared/artifact-style`, `znf:_shared/spec-template` and `znf:_shared/constitution`.

**Do not commit the spec, and do not `git add -f` it** — this project deliberately blocks it with
`.gitignore` instead. Three consequences to respect:

- Write it in the **main checkout**, never in the worktree — a worktree does not carry ignored,
  untracked files, and `wt rm` would delete it along with the branch.
- **Pass SDD absolute paths** to the spec and the plan. A relative path resolves against the
  worktree, where the file does not exist.
- `git clean -fdx` deletes every spec and plan. They are scratch, not history — so if a decision
  in there matters beyond this task, it belongs in a memory or in `CLAUDE.md`, not only here.

## Step 3: Ground the spec — before the plan, not after

Call **`Skill(znf:ground)`** again, now on everything the approved spec commits to: the fields,
endpoints and payloads it decided on, beyond what the request named.

**When the plan's diff will touch a backend query, `Skill(znf:explain-plan)` is mandatory here**
(shift-left, advisory) and again at Step 7 (`/ship`, with teeth) — it runs the two-tier DB-perf
gate (`zenify db-perf` + dynamic explain).

Ground all six categories, not just the DB:

- DB collections/tables and fields — **list the real names, never type one from memory**
- API endpoints and their request/response shapes
- Queue/event names and payload fields
- Library methods and their signatures (read installed types, not memory)
- In-repo code the plan calls into: signatures, exported symbols, component props, config
  keys — read the definition, not a call site
- Env var names — off the running process, not off `.env`

If grounding contradicts the spec, fix the spec first. Do not write a plan on top of it.

Do not proceed with any unverified name.

## Step 4: `/scout` — what depends on what the spec is about to change

Call **`Skill(znf:scout)`** once, here — it dispatches the `scout` agent, so the run leaves both
`Skill(znf:scout)` and `Agent(znf:scout)`. After the spec has decided what changes, before `writing-plans` locks
the File Structure.

**Dispatch it at the top of Step 3, not after Step 3 finishes** — while `Agent(znf:scout)` sweeps,
the main loop can do Step 3's grounding inline. Collect the scout report before writing the
plan, since the plan's File Structure depends on it.

Brief for a feature is **mixed**: part is new code nothing calls yet, part plugs into code that
already has consumers. Point the scout at the second part:

1. **who reads / writes / calls** the shared things the spec touches — in this workspace,
   delegate that to `/gate` rather than re-deriving the eight-repo sweep
2. **which tests cover** the code the plan will modify
3. **what else is written in the same operation** — a queue job, a cache entry, a search index
4. **why the existing code is the way it is**, for anything being changed rather than added

If the report says **"cannot enumerate by grep"**, carry that word "partial" into the plan.
Do not launder a partial map into a clean one.

> Why: see `references/grounding-and-scout-rationale.md` — why Step 1 runs before brainstorming
> (`chatbot_setting`), Step 3 before `writing-plans` with no DB delegate, why 3 → 4.

## Step 5: Plan (`znf:writing-plans`)

Call **`Skill(znf:writing-plans)`**. Map the files first, then write tasks containing real code — no "TBD", no "add error
handling", no "similar to Task N". Run its self-review (spec coverage / placeholder scan /
type consistency). Save to `<main-checkout>/docs/superpowers/plans/<filename>.md` — **the main
checkout, same reason as the spec** (rule #8), and hand SDD the absolute path.

**Apply the same artifact-quality rules as the spec** — `znf:_shared/artifact-style`, cited under
Step 2.

The plan follows the same discipline as the spec — `znf:_shared/constitution` and
`znf:_shared/spec-template`.

**Decide here, once, which tasks are worth a browser run — and write it into their definition of done.**
This is the *only* thing that triggers a per-task UI check; Step 6 does not infer it from file extensions.

The test is whether the task's **deliverable is something you look at**, not whether it happens to touch
a rendering file:

```
worth it        a new screen · a new component · a layout or grid change · a modal
                → "Done when … and `znf:ui-verifier` reports it renders correctly, with the
                   overflow measurement of the changed element against its container."
NOT worth it    a copy change · a colour token · a css file touched in passing · wiring an
                existing component to a new endpoint
                → say nothing; `/ship` step 4 still sees it at the end
```

**At this same point, decide the E2E journey.** If a task has a UI→BE flow that changes an entity's
state, write it into the definition of done: "Done when … and a `.znf/e2e/<journey>.spec.ts` passes
`zenify e2e lint` and `zenify e2e run` is green." See the `znf:e2e` skill. Same criterion as
visual: only when the deliverable is a business flow, not for every task.

**Then `Skill(znf:ground)` a third time, on any name the plan introduced.** Call it after the plan is
written, not inside `writing-plans` — that skill stays untouched.

That skill ends by offering an execution choice. **The answer is already fixed: always
Subagent-Driven. Do not ask.**

> Why: see `references/spec-and-plan-rationale.md` — nine-step recap, what `artifact-style`,
> `spec-template`, `constitution` require, traceability, cost of getting it wrong, why this
> belongs in the plan, its grounding pass, why `executing-plans` is unused.

## Step 5b: Inspect spec+plan (`znf:analyze`) — advisory

After the plan is done and **before** dispatching SDD, call **`Skill(znf:analyze)`** on the spec+plan
pair (absolute path, in the main workspace). It runs `zenify analyze` (coverage FR→task, leftover
markers, structural Brief) then adds judgment (SC-testable, necessity, db-3).

**This is advisory — it does NOT block.** If there's a CRITICAL/HIGH finding (orphan FR, leftover
marker), surface it and let the user decide: fix the spec/plan and rerun, or accept and continue. A
named line `Skill(znf:analyze)` must appear at this step; its absence = the step was skipped. The
command is fail-open, so this step should never itself break the cook flow.

## Step 6: Implement (`znf:subagent-driven-development`)

### First, the worktree — this is the step that writes code

> **Isolation & base-ref doctrine → znf:discipline §8** (single source): worktree is unconditional; the base is the repo's declared baseRef, read never hardcoded; fetch before resolving the base; the carve-outs live there. Below is only what `/cook` adds operationally at this step.

**A worktree, always — house rule #8, no conditions.**

```bash
git -C <repo> fetch origin                                    # belt-and-suspenders
cd <repo> && wt new <slug> --type feat --base "$(node -e 'console.log(JSON.parse(require("fs").readFileSync(".claude/worktree.json","utf8")).baseRef)')"
```

**Polyrepo:** one worktree per affected repo, **same slug** in every one.

When the plan spans repos, do not run them one after another by default. Hand SDD the whole set:
it dispatches **one implementer per repo in parallel**, gating a dependent repo on the other's
**contract-frozen** commit (not on its whole plan). See SDD's "Cross-worktree parallelism (polyrepo)" section.

**Re-entering `/cook` mid-task does not mean a second worktree.** If this feature already has one,
`cd` into it — `wt new` will refuse and print the path (house rule #8). The slug belongs to the whole
**plan**, fixed at the first `wt new`, and does not subdivide — every `Task 1..N` below shares this
one worktree per repo.

**Spec and plan stay in the MAIN checkout, and are already written by now.** The worktree holds
code only. Every path handed to SDD must therefore be **absolute**.

> Why: see `references/worktree-and-handoff.md` — refetch reason, SDD Setup's assumption, and
> what to do before a second worktree.

### Start the app once, if anything in the plan has to be exercised

Same trigger as everywhere else: **if any task's definition of done can only be shown by calling
the running app** — an endpoint, a tool a client invokes, a queue consumer, a socket event —
invoke `Skill(znf:run)` once, before the task loop, and keep it up for the whole run.

Nothing to exercise — a refactor fully covered by tests, a docs change — then skip it and say so.

### Then SDD

Call **`Skill(znf:subagent-driven-development)`**. Always SDD, at every size. Per task: brief → implementer (code + test + commit + self
review) → task reviewer (spec compliance **and** quality) → fix loop, capped at five
rounds with a scoped re-review each round → ledger line. Then one final whole-branch
review on the most capable model.

- Tell SDD the workspace created just above already exists; it should verify, not create.
- **Implementers: the least powerful model that can handle the task, on every dispatch** — SDD's
  rule. Plan carries the **complete code** → transcription → a **fast, cheap model**
  (`claude-haiku-4-5`); **from prose** or multi-file integration → `sonnet` + `effort: 'xhigh'`.
  **Never pair haiku with `xhigh`** — it is not xhigh-capable and the CLI silently downgrades it.
- Scale **up** to `opus-4-8` only for design judgment or broad codebase understanding — reach it by
  **omitting** `model`, never `'opus'` (see "Naming is asymmetric").
- Minimum code to satisfy the plan's definition of done. TDD: failing test → implement →
  pass. Match existing style; no upgrades to unrelated code.

### Parallel implementers: across repos yes, within one repo no

```
different repos (be / web / hub / subscriber)   separate histories already   → PARALLEL
same repo, same worktree                        review package contaminated  → NEVER
same repo, one worktree per implementer         safe, but N branches to merge → OFF by default
```

**Dispatch the cross-repo group in ONE message** — that is what makes them concurrent; one message
each runs them in sequence and buys nothing.

**With several implementers out at once, ask each for its report by name.** A lost report and a task
that finished quietly are indistinguishable from here, and only one is safe to build on
(`CLAUDE.md §3`). Do not write a ledger line for a task whose report never arrived.

**If the plan has tightly-coupled tasks, that is a plan defect — go back and re-decompose.**
SDD routes coupled tasks away from itself, but the answer is to fix the decomposition, not
to switch executor.

### The ledger's "review clean" does not cover appearance

The receipt for this step is SDD's ledger — `<repo-root>/.znf/sdd/<plan>/progress.md`, one
`Task <N>: complete (commits a1b2c3d..d4e5f6a, review clean)` per task.

> Why: see `references/step6-implementation-notes.md` — what "review clean" does not certify.

**So label it honestly.** A task with no visual verdict gets
`Task <N>: complete (commits …, review clean — appearance not checked)`.

### A task the plan flagged gets looked at before its ledger line is written

**The trigger is the plan, and only the plan.** If the task's definition of done asks for a `znf:ui-verifier`
verdict (Step 5 decided that), then after the task reviewer passes and **before** appending
`Task <N>: complete`, dispatch `znf:ui-verifier` scoped to **that task's deliverable only**, not the
whole feature. Its verdict joins the ledger line.

**This check does not parallelise, even when the implementers around it do.** The Playwright
browser is a single shared instance, so if two repos' tasks are running concurrently and both are
flagged, their verifier runs go **one after the other** — and the main session must not touch
Playwright while either is running.

**`Skill(znf:run)` first, and take the URL from it.** `/run` reads the port this worktree was
allocated, starts the server in a pane beside the agent, and reports the URL.

No such line in the brief → no browser run.

> Why: see `references/step6-implementation-notes.md` — why `Skill(znf:run)` here, review
> contamination, deploy order, plan-defect, serialisation.

## Step 6b: Inspect test-traceability (`znf:standards`) — advisory

After SDD finishes implementing (Step 6) and **before** `/ship`, call **`Skill(znf:standards)`** on
the spec + plan + root worktree. It cross-checks each FR against a real test on disk: a requirement
that was implemented but has no test (`untested-fr`), a declared test file that's missing
(`missing-test-file`), or an empty test file (`empty-test-file`). This is the cross-cutting picture
that SDD's per-task review does not give — it only sees one task, not "does every FR have a test".
Advisory: surface findings for the user to decide, does not block. A `Skill(znf:standards)` line is
the evidence this step ran.

## Step 7: Pre-ship gate

Run `/ship`: lint + build → the project's contract gate → behavioural verification → the contract review
lens → deploy order → commit + push the feature branch, then **open the PR** (never merge).

**`cat` `/ship`'s board file — do not retype or summarise it.** The gate writes it to
`${TMPDIR:-/tmp}/ship-board-<fp10>.md` and hands you the path; end this step by running
`cat <that-path>`.

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
| 6 Implement | subagents via SDD | `claude-haiku-4-5` when the plan carries complete code; `sonnet` from prose / integration; omit `model` for design-judgment | `xhigh` on sonnet+; haiku default (not xhigh-capable) |
| 6 Fix loop r1-3 | resume the same implementer | unchanged | as dispatched |
| 6 Fix loop r4-5 | fresh implementer, +1 tier | omit `model` → `opus-4-8` | **`xhigh`** |
| 6 Task review | subagents via SDD | `sonnet`, or omit for a high-risk diff (SDD's rule) | default |
| 6 Final review | subagent via SDD | omit `model` → `opus-4-8` (the ceiling) | default |
| 6 UI check, flagged tasks | `znf:ui-verifier` agent | sonnet (pinned) | default |
| 7 Ship review | `code-reviewer` agent | **pass `model` explicitly, scaled to the diff** — see `/ship` step 5 | default (`high`) |
| 7 Ship UI check | `znf:ui-verifier` agent | sonnet (pinned) | default |

> Why: see `references/step6-implementation-notes.md` — why no separate review, why `cat` not
> summarise, delegation, ship reviewer's scaling rule.

**Naming is asymmetric:** scale down = pass `model: 'sonnet'` (or `'claude-haiku-4-5'`); scale up = **omit `model`**.

> Why: see `references/step6-implementation-notes.md` — floor/ceiling rationale, alias-override
> mechanism, SDD's explicit-model assumption.

## References

Materialized at `~/.claude/skills/znf/skills/cook/references/`. Read a file only when its trigger fires.

- `references/why-no-triage-and-named-lines.md` — read when you want to call something "too simple for /cook" or skip a named line.
- `references/base-ref-archaeology.md` — read when the checkout is on someone's branch and you are unsure what to read.
- `references/grounding-and-scout-rationale.md` — read when Step 1/3/4 feel redundant.
- `references/spec-and-plan-rationale.md` — read when you consider skipping the spec or changing the execution choice.
- `references/worktree-and-handoff.md` — read before a second worktree, or when SDD Setup wants to create its own.
- `references/step6-implementation-notes.md` — read when you want to change a model tier, run implementers in parallel, or skip the per-task UI look.

## Constraints preserved from house rules

- No commit before the gate passes — verify first, then commit (rule #3)
- No PR creation, no merge (rule #7)
- No push to deploy branches (rule #7)
- Correctness-first: no `--fast`/minimal-planning mode
