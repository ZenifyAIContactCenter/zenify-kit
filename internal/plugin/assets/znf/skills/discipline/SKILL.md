---
name: discipline
description: The house rules for how to work in this workspace — routing by blast radius, no fabrication, verify before claiming done, worktree isolation, memory habits, planning, git safety, grounding external facts, and bounded research fan-out. The session digest names these; invoke this skill for the full text and rationale.
allowed-tools: Read Grep Glob Bash
---

# Discipline — standing working rules

Standing rules; they override default eagerness. Compressed; unabridged in `references/rules-full-text.md`.

Tool names for each action: `znf:_shared/harness-tools` (harness mapping table).

## 0 — Route the work before changing files

Two **independent** dials; never collapse them — a one-line change can have huge blast radius.

**Dial A — blast radius → how much spine.** Runs whether or not a skill applies:

```
first    fetch the declared base before you ground or design (fetch, never pull; `git show <base>:<path>`)
then     worktree BEFORE the first edit (rule #8; no conditions) — one per TASK: already in one? cd there
+ touches a shared resource → the project's contract gate    + gate reports cross-repo → znf:ship (mandatory)
always   verify with real output before saying it works
```

**Dial B — what is unknown → which skill:**

| Unknown | Skill |
|---|---|
| what to build, or how — no agreed design | `znf:cook` (suggested, never auto-run — it has user gates) |
| it's wrong and I don't know why | `znf:fix` (just start it) |
| nothing unknown, but it's broken in production now | `znf:hotfix` (+ `znf:fix` steps 0-2); composes with fix, lands on the release base |
| **nothing unknown** — the user stated both the what and the why | no skill; spine only |

**Skip the skill only when all four hold:** one repo · no shared resource touched · the user stated both what and why · not broken in production. "No skill" never means no branch, no gate.

> Rationalisations and their realities: `references/routing-red-flags.md`.

## 1 — No fabrication (don't guess)
- Do **not** reference a file, function, field, column, key, or endpoint you have not read this session. Unsure → grep/read first. Never assert from memory.
- In dynamically-typed code, verify a data field/column/key against the **real source** (DB document, payload, or the defining code) before using it.
- Non-obvious claims cite `file:line`. When you don't know, say so and check.
- **Multi-repo routing:** identify the target repo before searching or editing — repo map first, then a parallel keyword search across repos; ask the user as a last resort.

## 2 — No overthinking (smallest thing that works)
- Make the **smallest diff** that satisfies the request. Match the surrounding altitude and style; no unasked error-handling, abstraction, config, options or files. Prefer editing an existing file. Greenfield: scaffold the minimum that runs.
- **Context hygiene:** a tool result over ~50 KB goes to a file, then read the part you need. A file read once this session (memory index, roadmap, spec) is not read again — cite it. SDD briefs/review packages come from the skill's scripts; the controller never opens templates.
- Prefer the repo's own **coding skill** set (under `.claude/skills/`) for the stack idiom before improvising.

## 3 — Verify before claiming done
- Never say "done / fixed / works" without evidence: state what you ran and the result; say plainly what was skipped.
- Strongest means the stack allows: build/typecheck; run or trace the real path; grep the boundary for contract changes. **No tests → produce output from the real code path** (a throwaway script, or `znf:run`). "It compiles" is not behaviour.
- **UI changes: verify the LOOK** — screenshot AND measure the changed element against its own container box (`getBoundingClientRect`); page-level scroll is NOT enough.
- **Collect every dispatched report by name; silence is not a clean result** — a missing report makes the step **incomplete**, never a checkmark. Independent agents go in **one message**.

## 4 — Memory habit
- **Save when checkable:** (a) you had to read/query/run something to learn it, or (b) the user corrected you. Save the **rule, not the event**; never what code/git/docs state. Prefer no memory to a doubtful one.
- **The index is an index, never content** (its tail is dropped silently). **Update, don't accumulate** — two memories are never both authoritative on one subject.

## 5 — Plan before non-trivial work
- **Track multi-step work in the skill's ledger file** (SDD `progress.md`, ship board), never the harness task list: each list call is an extra API turn. **Name every dispatched agent there; tick only with its report in hand.**
- Non-trivial feature → design/plan and get agreement before coding. Trivial change → just do it.

## 6 — Balance
- "Fast but correct": ceremony at risky boundaries (data fields, contracts, irreversible); trivial changes stay small.

## 7 — Git safety
- **Feature branches: commit + push freely** after verifying; atomic, per repo convention. On a deploy branch, branch off first.
- **Deploy branches: never commit, push, or merge into them** (per-project list; a git-guard hook enforces it — if blocked, tell the user).
- **Open the PR (`gh pr create`), never merge** — even one you opened; merging is the user's deploy decision. Never force-push unasked.

## 8 — Isolate every code change in a worktree

**About to change code in a git repo? Work in a worktree. No conditions, no judgement.** The main checkout stays clean: reading, running, holding the record.

- **One worktree per slug, not per edit** — sub-tasks and follow-up fixes share the one already open (take the `cd` the tool prints). A hotfix is exempt (different base ref).
- **Base = the repo's DECLARED base ref, never hardcoded**; a hotfix overrides it to the latest release ref, resolved *after* `git fetch origin`. **The fetch is not optional.**
- **Polyrepo:** one worktree per affected repo, same slug in all of them.
- **Spec and plan files live in the MAIN checkout, never in the worktree**; hand subagents **absolute** paths.

> Commands, why unconditional, and the carve-outs: `references/worktree-rationale.md`.

## 9 — External-world facts: search first, label the source

- Twin of rule #1 for *outside-world* facts. Memory is stale by construction and the dangerous case is **confident staleness**, so the trigger is the **category**, not self-doubt: library/tool API, version or flag, "latest / as of", pricing, release notes, anything datable after the cutoff, a named entity or error string not read this session → **search or fetch first.**
- **Cite only fetched text** (label the URL); anything else is unverified, from memory. Skip the search for facts derivable from the repo or this session.

## 10 — Fan-out is the default for DECOMPOSABLE research, bounded

- Independent sub-questions → **spawn parallel agents proactively, one message.** Scale the count to complexity (fact-find = 1 · comparison = 2–4 · complex = 3–5+); fan out only when value beats the token cost.
- Track each dispatch by name and collect each (rule #3): silence ≠ a clean result.
- **Cap the return**: every dispatch names it — at most 40 lines back; tables, long lists and logs go to a file (scratchpad or `docs/reference`) and the reply carries the path.

> Full triggers and the fan-out evidence: `references/external-facts-and-fanout.md`.

## References

Read a file only when its trigger fires.

- `references/worktree-rationale.md` — you are about to skip the worktree, or the tool refuses and you want to know whether a carve-out applies.
- `references/agent-dispatch-notes.md` — a dispatched agent went quiet, or the task-list tool is missing from the harness.
- `references/routing-red-flags.md` — you are explaining to yourself why a routing step does not apply here.
- `references/rules-full-text.md` — you need the unabridged wording of § 0–§ 7 (this file is the compressed form).
- `references/external-facts-and-fanout.md` — you are about to state a library/version/pricing fact from memory, or to decide how many agents to spawn.
