---
name: subagent-driven-development
description: Use when executing implementation plans with independent tasks in the current session
---
<!-- Vendored from obra/superpowers (MIT). Adapted for the znf plugin. -->

# Subagent-Driven Development

Execute a plan with a fresh implementer subagent per task, a task review after each, and a whole-branch review at the end — here standalone, in `/ship` under `/cook`.

Tool names for each action: `znf:_shared/harness-tools` (harness mapping table).

- **One short narration line** between tool calls; the ledger carries the record. **Never check in between tasks** — only the four stops below, or all tasks done, end a run.
- **Rulings, not stalls.** Decide conflicts, ambiguities, plan defects and caps yourself, spec binding, ledgering each as `Ruling: <decision> — <why> — <cost if wrong>`.
- **Four things stop you, and only these:** an irreversible or destructive operation; a security-sensitive action; a side effect outside this worktree you would normally ask about (merge, push to a shared branch, publish); a plan too broken to act on.

## Setup

Work in an isolated workspace (`wt new`, znf:discipline §8), or verify the existing one. Never implement on main/master without explicit consent.

- **Each plan owns a workspace:** `scripts/sdd-workspace PLAN_FILE` prints its git-ignored directory (`<repo-root>/.znf/sdd/<plan-basename>/`), holding this plan's artifacts alone.
- **Track progress in a ledger file, not in todos**, at `<workspace>/progress.md`, first line `# SDD ledger — plan: <plan file path>`. **Resume from it**: `Task <N>: complete` is never re-dispatched; a task ending in a fix round resumes there.
- **Read the plan once** and the Spec it names (conflicts resolve against the spec), note the Global Constraints, then **scan for conflicts before Task 1**, ledgering the table with a ruling per row (`references/setup-rationale.md`).

## Model Selection

Least powerful model that can do the role, and **name the model on every dispatch** — `zenify up` sets `CLAUDE_CODE_SUBAGENT_MODEL=sonnet`, so an unnamed one lands there. Cheapest tier for mechanical work, standard for integration and judgment, most capable for design and the final review; rounds 4-5 a tier up (`references/model-selection.md`).

## The Task Loop

- **Within one repo tasks stay sequential — never two implementers in the same worktree. Across repos they run in parallel ("Cross-worktree parallelism")**: one implementer per repo, gated on `contract-frozen`, one repo-tagged ledger; collect each stream by name, silence = incomplete (`references/polyrepo-parallelism.md`).
- **Hand artifacts over as files**; batch same-shape tasks into one brief.

### 1. Dispatch the implementer

- **Record BASE** (`git rev-parse HEAD`) first — the review package and fix diffs need it.
- **Task brief:** `scripts/task-brief PLAN_FILE N` writes the task's text to a file and prints the path. It is the single source of requirements, exact values live only there, and no subagent reads the whole plan (`references/dispatch-brief-contract.md`).
- **Report file** named after the brief (`…/task-N-brief.md` → `…/task-N-report.md`): the implementer writes its report there, returning only status, commits, a test summary, concerns.
- One dispatch, one task. The implementer dispatches nothing — not helpers, never a reviewer. Record its agent identity (rounds 1-3 resume it); never two implementers in one worktree.

Use the template in [implementer-prompt.md](implementer-prompt.md) verbatim.

### 2. Handle the report

Four statuses. **DONE:** generate the review package (`scripts/review-package PLAN_FILE BASE HEAD`, never `HEAD~1`) and dispatch the task reviewer with its path. **DONE_WITH_CONCERNS:** read them first. **NEEDS_CONTEXT:** supply it, re-dispatch. **BLOCKED:** route per `references/dispatch-brief-contract.md`. Never ignore an escalation or force a retry unchanged.

### 3. Review the task

Task-scoped gates. Never skip one, never accept a report missing either verdict — spec compliance AND task quality. Self-review never replaces one.

Hand the reviewer its diff as a file (`scripts/review-package PLAN_FILE BASE HEAD` at the recorded BASE, never `HEAD~1`, which truncates multi-commit tasks) with the brief, the report file and the binding global constraints verbatim. Resolve its "⚠️ Cannot verify from diff" items before completing the task; a confirmed gap is a failed spec review.

Use the template in [task-reviewer-prompt.md](task-reviewer-prompt.md) verbatim.

### 4. The fix loop

Triggered by spec ❌, any Critical or Important finding, or a ⚠️ item you confirmed. Two exits:

- **Minor findings never enter the loop.** Ledger them (`Task <N>: minor (deferred): <one-liner>`); the whole-branch review triages them before merge.
- **A plan-mandated finding is yours to rule on**, spec binding, ledgered before you act.

Everything else enters the loop. A round is one fix dispatch plus one scoped re-review. **Five rounds maximum per task.**

- **Rounds 1-3 resume the original implementer** with the findings verbatim; **rounds 4-5 dispatch a fresh one a tier up**. Inputs and fix-report contract: `references/fix-loop-mechanics.md`.
- **The re-review is scoped:** `scripts/review-package PLAN_FILE FIX_BASE HEAD` (FIX_BASE = the head the previous review saw), dispatching [re-review-prompt.md](re-review-prompt.md) verbatim. Each finding returns ADDRESSED or NOT ADDRESSED; new breakage in the fix diff joins the open list, out-of-scope notes become deferred minors.
- **After each round,** append `Task <N>: fix round <R>/5 (<X> addressed, <Y> open — <finding one-liners>; commits <a7>..<b7>)`. Never fix findings yourself in the controller session.

**The breaker.** When round 5 still leaves findings open, stop dispatching and adjudicate each: park the contestable as `Task <N>: parked — <finding> — Ruling: <why>`; for a real load-bearing one, rule on the smallest unblocking change and ledger `Task <N>: Ruling: <finding> — <why>`. Silent discards are forbidden.

### 5. Complete the task

Review clean, or every open finding parked with a ruling at the cap → append:

- `Task <N>: complete (commits <base7>..<head7>, review clean)`
- `Task <N>: complete (commits <base7>..<head7>, <K> parked)` after a tripped breaker

Never start the next task with Critical/Important issues neither fixed nor parked with a ruling at the cap.

## Final Review

**Under `/cook`, skip this section and go to Finish** — `/ship` reviews the branch plus the ledger's `minor (deferred)` and `parked` lines via the ship-pack `## Deferred`. Standalone: `scripts/review-package PLAN_FILE MERGE_BASE HEAD` (MERGE_BASE = where the branch started), its path in the dispatch, the most capable model, [code-reviewer-template.md](code-reviewer-template.md) verbatim, pointed at the deferred and parked lines.

Findings → ONE fix subagent with the whole list, then exactly one scoped re-review of it ([re-review-prompt.md](re-review-prompt.md)), adjudicating the rest as in the breaker. There is no second fix wave — what remains surfaces when finishing-a-development-branch presents the options.

## Finish

Collect every ledger line containing `Ruling:` into your final message under "Rulings I made", in order, each with what it costs if wrong.

Standalone, once the final review is clean and its fixes merged, delete the workspace (`rm -rf <workspace>`). **Under `/cook`, leave it** — `/ship` reads the ledger.

Use znf:finishing-a-development-branch.

## References

- `references/diagrams.md` — the process as a graph; when to use SDD.
- `references/example-workflow.md` — a first run, end to end.
- `references/setup-rationale.md` — the workspace, the scan, losing your place.
- `references/dispatch-and-review-rationale.md` — why each step is shaped so.
- `references/dispatch-brief-contract.md` — composing a dispatch or briefing.
- `references/fix-loop-mechanics.md` — running a fix round.
- `references/model-selection.md` — choosing a tier.
- `references/polyrepo-parallelism.md` — the plan spans repos.
