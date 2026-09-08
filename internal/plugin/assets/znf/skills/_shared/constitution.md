# Spec constitution — shared reference

**Last updated:** 2026-09-07

The governing principles every spec a skill authors must follow. Skills such as
`znf:brainstorming`, `znf:writing-plans`, and `znf:cook` cite this file at their
spec/plan-writing step instead of restating it. Format, prose language, and diagram rules are
NOT here — they live in `znf:_shared/artifact-style`; this file owns spec *discipline* only.

Rigor scales with the task (brainstorming's spike / bounded / architectural paths). The
**floor** below is uniform across every tier; the **form** — a full Brief, heavy clarify,
detailed checks — dials up at the architectural tier and nearly off for spike and bounded.

## Principles

- **P1 — Brief-first.** An architectural spec MUST open with the eight-field Brief
  (`znf:_shared/spec-template`).
- **P2 — Stable IDs.** Every functional requirement (FR) and success criterion (SC) MUST carry
  a stable ID (`FR-1`, `FR-1.1`, `SC-1`).
- **P3 — Testable SC.** Every SC MUST be checkable — Given/When/Then, or a single
  "the system shall" sentence. No prose-only success criteria.
- **P4 — Mark the unknowns.** Any unsettled point MUST be written as
  `[NEEDS CLARIFICATION: …]`, never silently assumed.
- **P5 — Clarify before the body.** Run the clarify pass on the Brief before writing FR/SC.
  Intensity scales with tier.
- **P6 — Necessity ladder (YAGNI).** Before a spec mandates a new component, dependency, or
  abstraction, justify it down the ladder: (1) does it need to exist, (2) is it already in the
  codebase, (3) does stdlib / the platform / an installed dependency cover it, (4) is one line
  enough — only then spec new code. A spec that mandates building what an existing path already
  covers is a defect. **Every deliberate cut or deferral records a *ceiling* (the known limit of
  the shortcut) and a *trigger* (the condition that reopens it).** A deferral with no trigger is
  the "later means never" failure. Lazy solution, never lazy analysis.
- **P7 — Safety floor.** A spec MUST NOT simplify away: input validation at trust boundaries,
  error handling that prevents data loss, security, accessibility basics (when there is UI), and
  — when it touches a database — the three DB guarantees (query-plan / no full collection scan,
  tenant-scope negative assertion, keyset not offset pagination); mark them N/A explicitly when no
  DB is touched. **Comprehension floor:** the ladder shortens the *solution*, never the
  *understanding* — a spec MUST trace the real flow and blast radius before proposing any cut. The
  dangerous laziness is laziness about understanding.
- **P8 — Traceability.** Every FR MUST be implemented by at least one plan task, and every task
  MUST name the FR/SC it serves. Plans mark this with a `_Requirements: FR-N[, SC-M]_` line per
  task. The IDs P2 assigns are only worth assigning if they flow through to the plan; without this
  link, coverage analysis has nothing to check. A commit that implements a spec SHOULD name the
  FR/spec it serves in its message, extending the same FR→task→commit chain (doctrine — no
  mechanism enforces it here).
- **P9 — Risk-metadata tags.** An architectural spec's Brief MUST carry three machine-readable
  line-start tags: `_Blast-radius:` (the repos/services that break — the blast radius the P7
  comprehension floor already demands), `_DB:` (the P7 DB guarantees, or `N/A`), and `_Rollback:`
  (how to undo the change, plus the prod-watch signal to check after deploy). These make the risk
  surface machine-checkable rather than prose-only. When the blast radius is cross-repo, the
  rollout MUST go through the contract gate (`/gate`) — a doctrine, not a mechanism here.
- **Format, language, diagrams:** follow `znf:_shared/artifact-style`. Not restated here.

## Governance

Edit this file like any other doc — git history is its version. Downstream `znf:` skills read it
at runtime; there is no cached copy to bump.

Where a learned thing belongs — which of the three layers (memory, CLAUDE.md, skill-rule /
constitution) it lives in, and when to promote or retire it — is decided by
`znf:_shared/knowledge-doctrine`. Read it before recording a rule at this layer: a principle added
here must be the lowest layer that still reaches every context where it must fire.

Where a *handoff* belongs — an ephemeral session-handoff versus a committed milestone-record — is a
sibling question, decided by `znf:_shared/handoff-doctrine`. Knowledge-doctrine places a learned
rule; handoff-doctrine places a record of session or milestone state.
