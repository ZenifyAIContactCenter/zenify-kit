---
name: cook
description: Full feature pipeline in one command. Use when implementing a feature end-to-end — branch, brainstorm to a spec, ground every name against real data, plan, subagent-driven implementation, then the pre-ship gate. Always spec-driven and always subagent-driven, at every size. Commits and pushes the feature branch on all-green (house rule #7); never opens the PR.
argument-hint: "<feature description or path/to/plan.md>"
allowed-tools: Read Grep Glob Bash Agent
---

**Every gate is kept, at every size.** No "small enough to skip" path.

Tool names for each action: `znf:_shared/harness-tools` (harness mapping table).

```
0 fetch → 1 ground → 2 brainstorm→spec → 3 ground → 4 scout → 5 plan → 6 wt+SDD → 7 /ship
```

- Worktree at **Step 6**, not Step 0.
- **No complexity triage** — scale the design, never the step count.
- **A `.md` path argument** → Step 0, ground **every name the file uses**, then Steps 4 and 6; skip 1, 2, 5. SDD's Setup resumes from the ledger: a `Task <N>: complete` line is not re-dispatched. **A description** → all steps.
- **Every step leaves a named line** — `Skill(znf:ground)` ×3, `Agent(znf:scout)` ×1, `Skill(znf:brainstorming)`, `Skill(znf:writing-plans)`, `Skill(znf:subagent-driven-development)`, `Skill(znf:ship)`. A missing line is a skipped step.

## Step 0: Sync the base

```bash
git -C <repo> fetch origin                    # per affected repo
node -e 'console.log(JSON.parse(require("fs").readFileSync(".claude/worktree.json","utf8")).baseRef)'
git -C <repo> rev-list --count HEAD..<baseRef> # how far behind the checkout is
```

**`fetch`, never `pull`; never switch the checkout's branch.** Report per repo: base · on it · behind · dirty.

## Step 1: Ground the request — before brainstorming

**`Skill(znf:ground)`** on the entities the request names, before any clarifying question; field detail waits for Step 3.

```bash
zenify db-read collections <term-from-the-request>   # real names
zenify db-read doc <a-name-from-that-list>           # real fields
```

## Step 2: Brainstorm → spec (`znf:brainstorming`)

**`Skill(znf:brainstorming)`**, its nine steps as written, **two real user gates**; keep polyrepo scope (repos, contracts, what breaks). Spec and plan follow `znf:_shared/artifact-style`, `znf:_shared/spec-template` and `znf:_shared/constitution`. **Never commit the spec or `git add -f` it**: it lives in the **main checkout**; hand SDD **absolute** paths.

## Step 3: Ground the spec — before the plan, not after

**`Skill(znf:ground)`** on everything the spec commits to — DB fields, API shapes, queue payloads, library signatures, in-repo symbols, config keys, env vars. **Real names only; read the definition, not a call site.** Contradiction → fix the spec. **A backend query in the diff makes `Skill(znf:explain-plan)` mandatory** here (advisory) and at Step 7 (with teeth).

## Step 4: `/scout` — what depends on what changes

**`Skill(znf:scout)`** once, dispatched at the top of Step 3 so `Agent(znf:scout)` sweeps while the main loop grounds inline; collect the report before the plan. Four targets: consumers of the shared things (delegate that sweep to `/gate`), covering tests, co-writes, why the code is as it is. **"cannot enumerate by grep"** → carry "partial" into the plan.

## Step 5: Plan (`znf:writing-plans`)

**`Skill(znf:writing-plans)`**. Files first, then tasks with real code — no "TBD", no "similar to Task N". Run its self-review. Polyrepo plans → SDD "Cross-worktree parallelism". Save to `<main-checkout>/docs/superpowers/plans/<filename>.md`.

**Decide per task, here and only here, whether its definition of done requires a `znf:ui-verifier` verdict** (renders correctly; overflow measured against its container) **or an E2E journey** — decide the E2E journey here (`.znf/e2e/<journey>.spec.ts` green under `zenify e2e lint` + `zenify e2e run`, `znf:e2e`). Step 6 infers neither.

**Then `Skill(znf:ground)` a third time**, on any name the plan introduced. `writing-plans` ends by offering an execution choice: **always Subagent-Driven. Do not ask.**

## Step 5b: Inspect spec+plan (`znf:analyze`) — advisory

Before dispatching SDD, **`Skill(znf:analyze)`** on the spec+plan pair (absolute path): FR→task coverage, leftover markers, Brief structure. **Advisory — it does NOT block**; surface CRITICAL/HIGH for the user to decide.

## Step 6: Implement (`znf:subagent-driven-development`)

> **Isolation & base-ref doctrine → znf:discipline §8** (single source). Below is only what `/cook` adds.

**A worktree, always — house rule #8, no conditions.**

```bash
git -C <repo> fetch origin                                    # belt-and-suspenders
cd <repo> && wt new <slug> --type feat --base "$(node -e 'console.log(JSON.parse(require("fs").readFileSync(".claude/worktree.json","utf8")).baseRef)')"
```

- **Polyrepo:** one worktree per repo, **same slug**; one implementer per repo, a dependent repo gated on the other's **contract-frozen** commit. Re-entering `/cook` is not a second worktree: `cd` into it.
- A definition of done only the running app can show → **`Skill(znf:run)`** once before the task loop, kept up.

**`Skill(znf:subagent-driven-development)`** at every size, told it runs under `/cook` and the worktree exists (verify, not create). **Under `/cook` SDD skips its own final review**: `/ship` step 5 reviews the branch and picks up the ledger's `minor (deferred)` and `parked` lines.

**A task the plan flagged — and only the plan — is verified before its ledger line:** after its reviewer passes and **before** appending `Task <N>: complete`, `Skill(znf:run)` for the URL, then `znf:ui-verifier` on **that deliverable only**; the verdict joins the line; these serialise on the one shared browser. Unlooked-at: `Task <N>: complete (commits …, review clean — appearance not checked)`.

## Step 6b: Test-traceability (`znf:standards`) — advisory

After Step 6 and **before** `/ship`, **`Skill(znf:standards)`** on spec + plan + root worktree: every FR against a real test on disk (`untested-fr`, `missing-test-file`, `empty-test-file`). Advisory.

## Step 7: Pre-ship gate

**`Skill(znf:ship)`**: lint + build → contract gate → behavioural verification → review → deploy order → commit + push, then **open the PR** (never merge). **`cat` the board file `${TMPDIR:-/tmp}/ship-board-<fp10>.md`; never retype or summarise it.**

## Which model runs which step

Steps 0–5 run in the **main loop** on Opus. Unlisted effort: default.

| Step | Runs as | Model / effort |
|---|---|---|
| 4 Scout | **`scout` agent** | sonnet (pinned in the agent definition) |
| 6 Implement | subagents via SDD | SDD Model Selection: `haiku` transcription → `opus` design judgment; `xhigh` on sonnet+ only |
| 6 Fix loop | r1-3 same implementer · r4-5 fresh, +1 tier | unchanged · `opus` **`xhigh`** |
| 6/7 UI check | `znf:ui-verifier` agent | sonnet (pinned) |
| 7 Ship review | `code-reviewer` agent | **explicit, scaled to diff**; effort `high` |

**Name the tier and the return cap on every dispatch** — no explicit model means sonnet (`CLAUDE_CODE_SUBAGENT_MODEL`), never the session model; ≤ 40 lines back, long output to a file.

## References

- `references/why-no-triage-and-named-lines.md` — triage; named lines.
- `references/base-ref-archaeology.md` — reading another branch.
- `references/grounding-and-scout-rationale.md` — the six categories; scout targets.
- `references/spec-and-plan-rationale.md` — spec rules; the worth-it test.
- `references/worktree-and-handoff.md` — a second worktree; SDD Setup's own.
- `references/step6-implementation-notes.md` — model tiers, parallelism.
