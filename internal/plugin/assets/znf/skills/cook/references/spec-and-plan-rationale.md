<!-- Moved verbatim from cook/SKILL.md § Step 2: Brainstorm → spec, § Step 5: Plan (W4 slim-skills).
Read when: you consider skipping the spec or changing the execution choice. -->

### From § Step 2: Brainstorm → spec (`znf:brainstorming`)

1. explore context → 2. visual companion, only just-in-time → 3. clarifying questions,
**one per message** → 4. two or three approaches with a recommendation, YAGNI →
5. present the design in sections, **user approves after each section** →
6. write the spec to `<main-checkout>/docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md` —
**the main checkout, not the worktree** (rule #8) →
7. spec self-review (placeholders / contradictions / scope / ambiguity) →
8. **user reviews the written spec file** → 9. hand off to `writing-plans`.

### From § Step 2: Brainstorm → spec (tier two)

When you write the spec/plan, follow `znf:_shared/artifact-style` (the shared reference: `**Label:**
value` headers, no markdown tables, no diagram-viewer-that-doesn't-render, title in the prose language
keeping jargon). See that file.

And follow spec discipline: write the spec per `znf:_shared/spec-template`, per the principles in
`znf:_shared/constitution` (Brief-first, FR/SC have IDs, SC testable, mark unclear spots,
necessity ladder, safety floor, traceability). artifact-style handles format; these two assets handle spec *content*.

### From § Step 5: Plan — traceability tail (extra, tier two)

`znf:_shared/constitution` and `znf:_shared/spec-template` flow requirement IDs into
`_Requirements:` task lines (traceability P8) and the necessity note guards over-engineering at
plan time. The plan is as much a human-read artifact as the spec.

### From § Step 5: Plan — the browser-run test (tier two)

Getting this wrong in the cheap direction is fine — a task you did not flag is still caught by the final
gate, just without attribution to a task. Getting it wrong in the expensive direction costs a browser run
per task plus a report to chase, on the pipeline you run most.

### From § Step 5: Plan — why not `executing-plans` (tier two)

`executing-plans` is not used here — it defers to SDD itself when subagents are available, and it
has no code review of any kind: no per-task reviewer, no fix loop, no final review.

### From § Step 2 — why the spec is gitignored, not committed (extra, tier two)

Step 6 of `brainstorming` says to commit the design document; this project deliberately blocks
it with `.gitignore` instead.

### From § Step 5: Plan (`znf:writing-plans`)

This belongs in the plan and not only in Step 6's rules, because `scripts/task-brief` extracts each
task's text **from the plan file** with `awk`, and SDD calls that brief *"the single source of
requirements"*. A requirement written into the plan therefore arrives in front of you at dispatch time;
a requirement living in this file has to be remembered while you are reading SDD's loop instead — and a
rule in the wrong file is exactly how `/ship` came to contradict this one until a cross-check caught it.

It has no grounding step of its own. Verified: its self-review checks the plan against the
spec, and against itself ("match what you defined in **earlier tasks**") — never against
reality. So a plan can be fully self-consistent, cover every spec requirement, carry no
placeholder, and have every field name wrong. `implementer-prompt.md` does not check either;
its only verification lines are about tests. Step 3 grounded what the *spec* committed to;
this covers what the *plan* added — and plans do add names, because this skill demands real
code in every task. Without this pass the only net left is the ship-pack's `## Ground` block
at `/ship`, which fires after the code is already written.

### The worth-it test for a browser run, and the E2E twin (moved from § Step 5, token diet 2026-09-18)

The test is whether the task's **deliverable is something you look at**, not whether it happens to
touch a rendering file.

```
worth it        a new screen · a new component · a layout or grid change · a modal
                → "Done when … and `znf:ui-verifier` reports it renders correctly, with the
                   overflow measurement of the changed element against its container."
NOT worth it    a copy change · a colour token · a css file touched in passing · wiring an
                existing component to a new endpoint
                → say nothing; `/ship` step 4 still sees it at the end
```

**The E2E journey uses the same criterion.** A task whose flow is UI→BE and changes an entity's
state — creating, editing or deleting a ticket, deal or contact — gets, in its definition of done:
"Done when … and a `.znf/e2e/<journey>.spec.ts` passes `zenify e2e lint` and `zenify e2e run` is
green." Only when the deliverable is a business flow, never for every task.

### Spec and plan are gitignored: the three consequences (moved from § Step 2, token diet 2026-09-18)

- Write both in the **main checkout**, never in the worktree — a worktree does not carry ignored,
  untracked files, and `wt rm` would delete them along with the branch.
- **Pass SDD absolute paths.** A relative path resolves inside the worktree, where the file is not.
- `git clean -fdx` deletes every spec and plan. They are scratch, not history: a decision that
  matters beyond this task belongs in a memory or in `CLAUDE.md`.
