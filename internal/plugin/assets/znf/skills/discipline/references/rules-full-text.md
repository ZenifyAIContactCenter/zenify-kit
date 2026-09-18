# House rules § 0–§ 7 — unabridged wording

The SKILL.md carries the compressed form of each rule; this file is the full text, kept for when a rule's exact wording matters. The red-flag table lives in `routing-red-flags.md`.

## 0 — Route the work before changing files

Two **independent** dials. Do not collapse them — a one-line mechanical change can have a huge blast radius, and a large design question can be confined to one repo.

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

`znf:hotfix` is on a different axis from the other two: it is about where the change lands (a release base ref, not the mainline integration branch), so it *composes* with `znf:fix` rather than replacing it.

**Skip the skill only when all four hold:** one repo · no shared resource touched · the user stated both what and why · not broken in production. Otherwise a skill applies. Note dial A still applies — "no skill" never means "no branch" and never means "no gate".

**`znf:cook` is suggested, never auto-run.** It has multiple user gates and writes artifacts; a wrong guess costs the user an interrupt mid-ceremony. Say in one line that the work looks like `znf:cook` and let them decide. `znf:fix` and the spine you just start.


### 1 — No fabrication (don't guess)
- Do **not** reference a file, function, field, column, key, or endpoint you have not read this session. Unsure → grep/read first. Never assert from memory.
- In dynamically-typed code (no compiler to catch a wrong name), verify a data field/column/key against the **real source** before using it: the actual DB document, the actual payload, or the code that defines it. The project's own docs say how to reach its real data.
- Non-obvious claims about the codebase must cite `file:line`. If you can't cite it, you're guessing.
- When you genuinely don't know, say so. "I'm not sure, let me check" beats a confident wrong answer.
- **Multi-repo routing:** identify the **target repo before searching or editing**, first via the repo map in the project's own docs. If the term isn't in the map, **auto-locate** it: dispatch a parallel search for the feature's *real* keywords across repos, never a sequential grep of the literal feature name. Ask the user only as a last resort.

### 2 — No overthinking (smallest thing that works)
- Make the **smallest diff** that satisfies the request. Nothing extra. Match the altitude and style of the surrounding code; don't "upgrade" unrelated code.
- Do not add error-handling, abstraction, config, options, or new files that weren't asked for. No speculative future-proofing. Prefer editing an existing file over creating one.
- **Context hygiene — three rules, measured 2026-09-18 (median 184k context per turn):** a tool result over ~50 KB goes to a file, then read the part you need. A file read once this session (memory index, roadmap, spec) is not read again — cite it. SDD briefs and review packages come out of the skill's scripts; the controller never opens the templates.
- For greenfield projects the same rule bites hardest: scaffold the minimum that runs, not a kitchen-sink boilerplate.
- When writing or changing code in a repo, prefer its repo-scoped **coding skill** set (installed under `.claude/skills/`) for the stack idiom before improvising.

### 3 — Verify before claiming done
- Never say "done / fixed / works" without evidence. State what you ran and the result. If something is untested or skipped, say so plainly.
- Verify by the strongest means the stack allows: build/typecheck for typed code; run or trace the real code path for untyped code; grep across the boundary for contract changes (shared collections, pub/sub payloads, HTTP shapes, queue jobs).
- **With no tests, produce output from the real code path and show it** — a throwaway script, or `znf:run` for app-level changes. "It compiles" is not verification of behaviour. If you could not execute it, say that instead of implying you did.
- **UI changes: verify the LOOK, not just the flow.** Anything that renders must be checked visually: a change can pass every behavioural check while the layout is broken. Capture a screenshot AND measure the **changed element against its own container box** (`getBoundingClientRect`: `child.right` vs `container.right − paddingRight`); page-level scroll (`scrollWidth==clientWidth`) is NOT enough, since a child can spill an inner panel without producing a scrollbar. Tell a browser-based verifier to return the overflow numbers and to compare against an unchanged sibling, so you know whether the spill is yours.
- **Ask for the report by name; never read silence as a clean result.** Track what you dispatched and collect each by name: a missing report makes the step **incomplete**, not clean — for a sweep, "no report" reads exactly like "no hits", and only one is safe to build on. Never write a checkmark, a ledger line, or "N things checked" for an agent that went quiet. Concurrency is still the right default for independent work: several agents in **one message** run at once.


### 4 — Memory habit
- **Save when one of two checkable things is true**, not when it feels "non-obvious": (a) you had to read a file, query the DB, or run something to learn it, or (b) the user corrected you on it. Save the **rule, not the event** — "the producer and consumer read the queue name from env, so the file on disk ≠ the running container" changes what to check next time; a dated bug fix changes nothing.
- **Never save** a one-off task conclusion, a narrative of what happened, or anything the code, git history or the project's docs already state. Prefer no memory to a doubtful one: a missing memory costs a re-derivation, a wrong one is asserted confidently every time it matches.
- **The memory index is an index, never content.** Only its first portion loads each session and the rest is dropped **silently**. One line per memory, written as the question that memory answers.
- **Update, don't accumulate.** Read the existing memory on that subject first: overlapping → edit it; contradicting → fix it. Two memories must never both be authoritative on one subject.

### 5 — Plan before non-trivial work
- **Keep a plan list for anything over ~3 steps and tick it as you go.** It is the only progress visible without reading every line of output. Two failure modes, both worse than no list: it stops being updated and then *asserts* a false state, or it exists on a one-step task and is pure noise. When a plan already has its own ledger, the list **mirrors** the ledger — never two sources of truth. **Any turn that dispatches subagents is over the threshold by itself.**
- The task-list tool is off by default on newer models; re-enable with `CLAUDE_CODE_ENABLE_TODO_TOOLS=1` set before the session starts.
- For a non-trivial feature, **design/plan before coding**: clarify intent, list affected files and contracts, get agreement. For trivial changes (a line, a string, a config value), skip the ceremony.

### 6 — Balance
- These rules serve "fast but correct". Read-before-write and verify-before-claim are required at risky boundaries (data fields, contracts, anything irreversible). For obviously trivial changes, keep the diff small and don't invent.

### 7 — Git safety
- **Feature branches: commit + push freely**, no need to ask. Verify first (rule #3), then commit atomically in the repo's existing convention (infer it from the commit log). On a deploy branch, branch off before you start.
- **Deploy branches: never commit, push, or merge into them.** Which branches deploy is a per-project fact declared in the project's config (fallback: main/master/staging/develop/production). A git-guard hook enforces it — if it blocks you, tell the user rather than working around it.
- **Open the PR, but never merge.** After pushing, open it yourself (`gh pr create`) with a conventional title and a body following the repo's template — the release report is derived per-PR, so a well-formed PR keeps the changelog clean. Report the URL and stop. **Merging is the user's deploy decision, including a PR you opened**: no `gh pr merge` (server-side, so no hook stops it), no merge into a deploy branch.
- Never force-push or rewrite pushed history without being asked.
