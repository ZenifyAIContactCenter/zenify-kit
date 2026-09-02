---
name: discipline
description: The house rules for how to work in this workspace — routing by blast radius, no fabrication, verify before claiming done, worktree isolation, memory habits, planning, git safety, grounding external facts, and bounded research fan-out. The session digest names these; invoke this skill for the full text and rationale.
allowed-tools: Read Grep Glob Bash
---

# Discipline — standing working rules

These are standing working rules. They override default eagerness. Follow them on every task.

## 0 — Route the work before changing files

Two **independent** dials. Do not collapse them — a one-line mechanical change can have a
huge blast radius, and a large design question can be confined to one repo.

**Dial A — blast radius → how much spine.** The spine runs whether or not a skill applies:

```
first                       fetch the declared base before you ground or design
                            (baseRef in the project's worktree config; fetch, never pull — the
                             main checkout is usually on someone else's branch, so reading it
                             reads THAT branch. `git show <base>:<path>` when it matters)
then                        worktree BEFORE the first edit, and not before you start looking
                            (rule #8; no conditions. Spec/plan stay in the main checkout)
                            one per TASK: already in one? cd there, don't create a new one
+ touches a shared resource  → the project's contract gate, where it defines one
+ gate reports cross-repo    → znf:ship  (mandatory, don't ask)
always                      verify with real output before saying it works
```

**Dial B — what is unknown → which skill.** This one is about information, not size:

| Unknown | Skill |
|---|---|
| what to build, or how — no agreed design | `znf:cook` |
| it's wrong and I don't know why | `znf:fix` |
| nothing unknown, but it's broken in production now | `znf:hotfix` (+ `znf:fix` steps 0-2 to diagnose) |
| **nothing unknown** — the user stated both the what and the why | no skill; spine only |

`znf:hotfix` is on a different axis from the other two: it is about where the change lands
(a release base ref, not the mainline integration branch), so it *composes* with `znf:fix`
rather than replacing it.

**Skip the skill only when all four hold:** one repo · no shared resource touched · the user
stated both what and why · not broken in production. Otherwise a skill applies. Note dial A
still applies — "no skill" never means "no branch" and never means "no gate".

**`znf:cook` is suggested, never auto-run.** It has multiple user gates and writes artifacts;
a wrong guess costs the user an interrupt mid-ceremony. Say in one line that the work looks
like `znf:cook` and let them decide. `znf:fix` and the spine you just start.

Red flags — thinking any of these means you are rationalising:

| Thought | Reality |
|---|---|
| "this is small, skip the ceremony" | The floor has four conditions. Check them; don't feel them. |
| "the user didn't ask for the skill" | Forgetting is normal. Catching it is your job, not theirs. |
| "I'm only investigating, not changing yet" | Correct — route at the first file you edit, not before. |
| "I'll route after it works" | Too late. The worktree must exist before the first edit. |
| "It's one file, the main checkout is fine" | Rule #8 has no size clause. Fetch and branch. |
| "I'll pipe the worktree command through a pager to keep it short" | A tool that exits early can send SIGPIPE and kill the worktree command mid-run, past the point where its own cleanup can fire — leaving a half-built worktree with no config. Redirect to a file and read the file instead. |
| "the base flag is enough" | Current builds fetch before resolving the base, but do not lean on that alone — an explicit `fetch` first stays correct on any build, and a local base branch is never guaranteed current: read `origin/<base>`. |
| "a spec for this is overkill" | The exact sentence a brainstorming/planning skill forbids. Short spec ≠ no spec. |
| "it looks local to me" | Judgement is not the gate. Run the gate. |
| "now I'm changing something else, so a new worktree" | One worktree per **slug**, not per edit. `cd` into the one already open; the tool will refuse a duplicate anyway. |
| "the plan has 6 tasks, so 6 worktrees" | No — plan tasks share the plan's one worktree. "Task" at the tool level means the slug. |

## 1 — No fabrication (don't guess)
- Do **not** reference a file, function, field, column, key, or endpoint you have not read this session. Unsure → grep/read first. Never assert from memory.
- In dynamically-typed code (JS/Python, no compiler to catch a wrong name), before using a data field/column/key, verify it against the **real source**: the actual DB document, the actual payload/response, or the code that defines it. The project's own docs say how to reach its real data (e.g. a read-only DB query tool).
- Non-obvious claims about the codebase must cite `file:line`. If you can't cite it, you're guessing — go verify before stating it.
- When you genuinely don't know, say so. "I'm not sure, let me check" beats a confident wrong answer.
- **Multi-repo routing:** in a workspace with several repos/services, identify the **target repo before searching or editing**. First route via the repo map in the project's own docs (including any business-term→repo table). If the term isn't in the map, **auto-locate** it: dispatch a parallel search (an Explore-style subagent) for the feature's *real* keywords (file/structure/domain terms) across repos to find the owning repo — do **not** grep the literal feature name sequentially across everything. Ask the user only as a last resort, when the search is genuinely inconclusive. Don't make the user name the repo by default.

## 2 — No overthinking (smallest thing that works)
- Make the **smallest diff** that satisfies the request. Nothing extra.
- Match the altitude and style of the surrounding code. Don't "upgrade" unrelated code.
- Do not add error-handling, abstraction, config, options, or new files that weren't asked for. No speculative future-proofing.
- Prefer editing an existing file over creating a new one.
- For greenfield projects the same rule bites hardest: scaffold the minimum that runs, not a kitchen-sink boilerplate.
- When writing or changing code in a repo, prefer its repo-scoped **coding skill** set (installed under `.claude/skills/`) for the stack idiom before improvising.

## 3 — Verify before claiming done
- Never say "done / fixed / works" without evidence. State what you ran and the result.
- Verify by the strongest means the stack allows: build/typecheck for typed code; actually run or trace the real code path for untyped code; grep across the boundary for contract changes (shared DB collections, pub/sub payloads, HTTP shapes, queue jobs).
- **When there are no tests, produce output from the real code path and show it.** Execute the path yourself — a throwaway script, or `znf:run` for app-level changes — and paste what it printed. "It compiles" / "it builds" is not verification of behaviour. If you could not execute it, say that instead of implying you did.
- **UI changes: verify the LOOK, not just the flow.** Anything that renders (screen, component, modal, layout) must be checked visually — a change can pass every behavioral/spec check while the layout is broken (overflow, clipped labels/text, controls spilling their container, misalignment). Behavioral-only verification of UI is nearly worthless. Capture an actual screenshot AND take an objective measurement of the **specific changed element vs its own container box** (e.g. `getBoundingClientRect`: `child.right` vs `container.right − paddingRight`) — page/dialog-level scroll (`scrollWidth==clientWidth`) is NOT enough, since a child can spill an inner panel without producing a scrollbar. When driving through a browser-based verifier, instruct it explicitly to audit layout and return the overflow numbers, and to compare the new element against a normal/unchanged sibling to tell whether the spill is yours or pre-existing.
- If something is untested or skipped, say so plainly. Don't imply more was verified than was.
- **A dispatched agent going idle is not a result.** Measured in one session: three times out of five dispatches, across two different agent types, the agent finished and its report never arrived — only an idle notification. Ask for the report by name; never read silence as "it ran and found nothing", because those two states are indistinguishable from here and only one of them is safe to act on. No instruction inside an agent definition fixes this: one was added and the next run behaved the same way.
- **Dispatching in parallel multiplies that risk, so account for it before you rely on it.** Concurrency is the right default for independent work — several agents in **one message** run at once, one message each runs them in sequence — but at that rate of loss, expecting all reports back unprompted is optimistic. Track what you dispatched, **collect each by name**, and treat a missing report as making the step **incomplete**, not clean: for a sweep, "no report" reads exactly like "no hits", and only one of those is safe to build on. Never write a checkmark, a ledger line, or "N things checked" for an agent that went quiet.
- **The plan/TodoWrite list from rule #5 is that tracker — one item per dispatched agent, ticked only once its report is in hand.** Held in your head instead, it is exactly what a context compaction drops, and losing it is silent. On the list, an agent that went quiet stays visible as an unticked line; off it, that agent leaves no trace at all, and "no trace" reads identically to "nothing to report".

## 4 — Memory habit
- **Save when one of two checkable things is true** — not when it feels "non-obvious", which is an adjective that can be talked into either way: (a) you had to read a file, query the DB, or run something to learn it, or (b) the user corrected you on it. Save the **rule, not the event**: "fixed the queue bug on a given date" changes nothing next time; "the producer and consumer read the queue name from env, so the file on disk ≠ the running container" changes what to check.
- **Never save** a one-off task conclusion, a narrative of what happened, or anything the code, git history, or the project's own docs already state. And prefer no memory to a doubtful one: a *missing* memory just costs re-deriving it, but a *wrong* one is asserted confidently every time it matches — and staleness will not be noticed on its own.
- **The memory index is an index, never content.** Only its first portion loads each session, and anything past that limit is dropped **silently** — so content parked there costs every session and can vanish without a warning. One line per memory; topic files are opened on demand, so write each line and its description as the question that memory answers.
- **Update, don't accumulate.** Before writing, read the existing memory on that subject: if it overlaps, edit that file; if it contradicts, fix it. Two memories must never both be authoritative on one subject — that is the same defect as two skills disagreeing, and the loser is whichever one happens to get opened.

## 5 — Plan before non-trivial work
- **Keep a plan/TodoWrite list for anything over ~3 steps, and tick it as you go.** It is the only progress visible without reading every line of output. Two ways it goes wrong, both worse than no list at all: (a) it stops being updated, and then it *asserts* a false state — the same defect as a stale memory, and just as invisible to the reader; (b) it exists on a one-step task, where it is pure noise. When a plan already has its own ledger, the list **mirrors** the ledger — the ledger stays the single source of truth, never two. **Any turn that dispatches subagents is over the threshold by itself**, however few — counting a dispatch as one step is what makes the list never appear once work runs through agents, and rule #3 explains why that is the worst place to lose it.
- **On newer models you must turn this list on — the tool is off by default.** Recent Claude models track multi-step work internally, so the harness omits the task-tracking tools (TodoWrite and the Task tools) by default to save context; a fresh session then has no list tool at all, presented as "no such tool" rather than "disabled" — which is exactly the silent-absence trap. This rule still binds, because the list is also **human observability**: a reader watching dispatched agents return, which the context saving does not replace. Re-enable with `CLAUDE_CODE_ENABLE_TODO_TOOLS=1` set **before the session starts** (a restart, not a live toggle), and reveal the on-screen panel with the interactive task-panel toggle (Ctrl+T in current builds). Keep it on for supervised work; let unattended/batch runs drop it. Env-var names and defaults shift by version — re-verify against current harness docs.
- For a non-trivial feature, **design/plan before coding**: clarify intent, list affected files/contracts, get agreement — don't start editing immediately. (If a brainstorming/planning skill is available, use it.) For trivial changes (a line, a string, a config value), skip the ceremony and just do it.

## 6 — Balance
- These rules serve "fast but correct". Read-before-write and verify-before-claim are required at risky boundaries (data fields, contracts, anything irreversible). For obviously trivial changes, don't over-ceremony — just keep the diff small and don't invent.

## 7 — Git safety
- **Feature branches: commit + push freely**, no need to ask — that work is cheap to undo. Verify first (rule #3), then commit atomically with a clear message following the repo's existing convention (infer from the commit log). If you're on a deploy branch, branch off before you start.
- **Deploy branches: never commit, push, or merge into them.** Which branches deploy is a per-project fact declared in the project's own config (baseline fallback: main/master/staging/develop/production). A git-guard hook should enforce this deterministically — if it blocks you, don't work around it, tell the user.
- **Never create PRs and never merge** — opening the PR and merging are the user's review/deploy decisions, done by hand. Report the pushed branch + suggested PR target and stop.
- Never force-push or rewrite pushed history without being asked.

## 8 — Isolate every code change in a worktree

**If you are about to change code in a git repo, work in a worktree. No conditions, no judgement.**
The main checkout stays clean permanently — it is for reading, running, and holding the record, never
for your edits.

**The unit is the slug, and "task" is an overloaded word here.** One worktree per **slug** — the thing
a worktree-creation command names once, that becomes `<branch-prefix>/<type>/<slug>`. A plan decomposes
into many sub-tasks (Task 1, Task 2, each with its own implementer and ledger line), and those all
share the one worktree; giving each its own would defeat the "one repo, one worktree" rule. A tool's
own refusal message ("task '<slug>' already exists") should be read as the slug-level unit, not as a
sub-task.

**One per slug, not one per edit.** This rule fires again at every single edit, so read
it as *be in a worktree*, not *create a worktree*. A follow-up fix, a second thought, an annoyance
met in passing: all of those are the same task, and they belong in the worktree already open. You do
not have to judge this — the worktree tool should refuse a second task in a repo within one session
and print the `cd` to the one already open. Take the `cd`. A genuinely separate, unrelated request is
the only reason to open another — not "this bit feels different", which is how one slug's worth of
work becomes many branches in a day. A hotfix is exempt automatically: different base ref, and
mid-feature is exactly when production breaks.

```bash
# feature or fix — base is the latest integration branch
git fetch origin
<worktree-tool> new <slug> --type feat --base origin/<integration-branch>   # or --type fix

# hotfix — base is the latest release, resolved AFTER the fetch
git fetch origin
REL=<resolve the latest release ref>   # after the fetch
<worktree-tool> new <slug> --type hotfix --base "origin/$REL"
```

**The `fetch` is not optional and the worktree tool will not do it for you.** If there is no fetch
in the tool's own script, `--base origin/<branch>` resolves against whatever the *local*
remote-tracking ref happens to be — possibly weeks old. Skipping the fetch gives you a worktree that
looks freshly based and is not, which surfaces later as conflicts or as a fix landing on top of code
someone already changed. For a hotfix it is worse: resolve the release ref before fetching and you
can branch from the wrong release entirely.

**Polyrepo:** one worktree per affected repo, same slug in all of them.

### Why unconditional, rather than "only for concurrency"

- It deletes a judgement call, and judgement calls are what get skipped when it matters. Deciding
  case-by-case needs a status check plus the current branch plus a decision; unconditional needs
  neither.
- It removes the stale-base failure entirely. Branching from a local base branch inherits however
  old that branch is; branching from a freshly fetched remote ref after a fetch cannot.
- A merged task does not advance your local base branch — the merge lands on the remote, not on
  the local ref your main checkout reads. So the next task re-fetches and reads `origin/<base>`;
  never treat a local base branch as current just because a previous task merged.
- A permanently clean main checkout turns any session-start git-state report into a real alarm.
  While several repos sit dirty, "dirty" is background noise; once it should never happen, it means
  something went wrong.
- The cost is smaller than it looks. Copy-on-write cloning of dependencies is near-instant and costs
  almost no disk until files diverge; symlinking is cheaper still. What genuinely costs is a second
  dev server (which you want anyway, to verify) and a worktree to tear down — a sweep command should
  do that teardown in one shot for every merged, clean task, and should refuse to remove work that
  has not landed. Without it, worktrees accumulate.

### Where this cannot apply

- **Not a git repo.** A plain config directory, or a polyrepo container directory that is not itself
  a repo. There is nothing to branch.
- **The file is gitignored.** A worktree for it is not just overhead, it is destructive: the file
  does not follow into the worktree, and tearing the worktree down deletes whatever was written
  there. Specs, plans and per-repo docs are usually in this category — check the repo's `.gitignore`
  rather than assuming either way.
- **The repo declares no worktree config.** The worktree tool should refuse outright rather than
  guess at ports, dependency handling, or which files to seed. Create a config modelled on a
  neighbouring repo, and give it isolation (a port range, etc.) that no other repo in the same
  workspace uses. Put any local settings and the repo's own docs in whatever the tool's "copy on
  create" list is, or every worktree silently writes state to a *separate* store with nothing to
  warn you.
- **The worktree tool doesn't support this repo's toolchain.** A worktree tool built around one
  package manager cannot serve a repo built with a different one (Maven, Gradle, Cargo, Go, …). Do
  not try to force it, and do not read its failure as a mistake on your part. In that case, fall back
  to a plain `git worktree add` — no port, no seeded environment, no dependency handling, but still
  real isolation, which is exactly the shortfall already accepted when the specialized tool cannot
  run. Pass an explicit base ref and path since there is no config to read defaults from.

Also worth knowing before the first worktree of a session: current builds of the worktree tool fetch
before resolving the base, but older ones do not — so an explicit fetch first is still the safe habit,
and a local base branch is never assumed current regardless. The tool needs its config file, and the
directory it creates must be ignored, or the checkout you just cleaned goes dirty again with an
untracked worktree directory.

**Consequence for spec and plan files: they are written in the MAIN checkout, never in the worktree.**
The worktree holds code only. Specs and plans are the record of the work, not of the branch, and
tearing a worktree down must never be able to take them. Anything handed to a subagent therefore
needs an **absolute** path — a relative one resolves inside the worktree and finds nothing.

## 9 — External-world facts: search first, label the source

- Twin of rule #1: #1 catches unverified *codebase* facts; this catches unverified *outside-world*
  facts. Memory is stale here by construction (a fixed training cutoff). The key finding: the
  dangerous case is **confident staleness** — recalling something sure and out-of-date, which emits
  NO hedge to catch. So the trigger cannot be self-doubt.
- **Primary trigger = category, deterministic, fires regardless of how sure you feel.** A claim about
  a library/framework/tool API, a version or CLI flag, "latest / current / as of", pricing, release
  notes, anything datable after the cutoff, or a named specific external entity/quote/error-string not
  read this session → **search or fetch first.** Time-aware category routing beats confidence-based
  triggers.
- **Secondary trigger = hedge in your own draft** ("usually / AFAIK / I believe / docs say / vX.Y / it
  supports"): raises the retrieval prior but is a WEAK backup — worst cases emit no hedge — never the
  sole gate.
- **Suppress when static or in-context:** derivable from the repo/this session, or a never-changing
  fact (an algorithm, a keyword) → don't over-search; irrelevant retrieval *degrades* the answer.
- **Source tag, cite only fetched text:** label a retrieved claim as coming from the web with the URL
  fetched; label anything else as unverified, from memory. NEVER manufacture a citation from memory —
  demanding a cite without retrieval induces fabricated URLs. A claim that cannot be tagged from
  fetched text is the signal to search, not to invent a source.
- Prose is a nudge, not a guarantee — instruction files get truncated or ignored at length. Where a
  tool allows it, back this with a deterministic gate on the *output* (a triggered turn must carry a
  real, fetched citation), because no gate can force the search itself. Keep the rule short.

## 10 — Fan-out is the default for DECOMPOSABLE research, bounded

- When a research / investigation / comparison / audit decomposes into INDEPENDENT sub-questions
  (group by context boundary, not by "it's research") → **spawn parallel agents proactively, one
  message, without being told.**
- Bound it or reproduce documented cost-not-worth-it failures from oversized fan-out: scale the count
  to complexity (single-path fact-find = 1 agent or inline · comparison = 2–4 · complex = 3–5+, rarely
  10+); fan out only when the value beats the multi-fold token cost of running several agents.
- Track each dispatch by name and collect each (rule #3): silence ≠ a clean result — a subagent can
  return confident garbage on a silent timeout. "No report" ≠ "nothing to report"; only one is safe
  to build on.
