# Spec template — shared reference

Copy this structure when authoring an architectural spec. Skills cite it from their
spec-writing step. Follow `znf:_shared/constitution` for the principles and
`znf:_shared/artifact-style` for format, prose language, and diagrams. Write prose in the
project's own language (see the project's convention); the scaffolding here is guidance only.

Any unsettled point uses the marker `[NEEDS CLARIFICATION: …]`.

## Brief

Seven fields, in order. The **floor** fields — required at every tier — are 1, 2, 5, 6. Fields
3, 4, and 7 are filled in full only at the architectural tier.

1. **Problem.** What is wrong or missing today, and the consequence. (floor)
2. **Approach.** How you will fix it — and a necessity check: *which existing path already
   covers this, and why it is not enough* (constitution P6). (floor)
3. **Timing.** Sequencing, dependencies, resumability.
4. **Phase.** Where this sits in the milestone breakdown.
5. **Service / blast-radius.** Which services or repos this touches; what breaks if the shape
   changes. (floor)
6. **DB guarantees.** query-plan (no full collection scan) · tenant-scope negative assertion ·
   keyset (not offset) pagination; write **N/A** explicitly when no DB is touched. (floor)
7. **Flow.** A diagram following `znf:_shared/artifact-style` (its conditional-diagram rule); do
   not hardcode a diagram syntax here.

**Mini-brief (bounded tasks).** A bounded change does not fill all seven fields. Use three lines:
Problem · Approach + necessity · Blast-radius + DB.

## Goals / Non-goals

State the goals, then the non-goals. **Each non-goal records a ceiling and a reopening trigger**
(constitution P6), for example:

    Deferred: <what>. ceiling: <known limit of leaving it out>. trigger: <when to revisit>.

A deferral with no trigger is the "later means never" failure.

## Functional requirements (FR)

A user story plus numbered acceptance criteria, each with a stable ID:

**FR-1.** As a <role>, I want <X>, so that <Y>.
- FR-1.1: <acceptance criterion>
- FR-1.2: <acceptance criterion>

*Optional EARS note* — for anyone who wants a keyword form, two shapes cover most cases:
`WHEN <event> the system SHALL <response>` and `IF <condition> THEN the system SHALL <response>`.
Not required; SC in Given/When/Then form is enough.

## Success criteria (SC)

Each with a stable ID, and testable:

**SC-1.** Given <state>, When <action>, Then <observable outcome>.

## Testing

For any non-trivial logic (a branch, a loop, a parser, a money or security path), name the
**smallest check that fails if the logic breaks**. A spike or a one-liner needs none. No
Testing-Strategy prose is required.
