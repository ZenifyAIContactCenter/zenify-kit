---
name: brainstorming
description: "You MUST use this before any creative work - creating features, building components, adding functionality, or modifying behavior. Explores user intent, requirements and design before implementation."
---
<!-- Vendored from obra/superpowers (MIT). Adapted for the znf plugin. -->

# Brainstorming Ideas Into Designs

Turn ideas into designs and specs through dialogue: classify how much process the request needs,
then work your path to your human partner's approval.

<HARD-GATE>
Do NOT invoke any implementation skill, write any code, scaffold any project, or take any
implementation action until you have told your human partner what you intend and they have approved
it. This applies to EVERY task on EVERY path below — the ceremony scales with the task; the
approval gate never does. A reply approves the stage actually presented: design approval only
permits writing the spec, spec approval only permits writing the plan, plan approval permits
implementing. Approving an idea or scope approves no artifact that does not exist yet. Resume at
the earliest incomplete stage.
</HARD-GATE>

## Three Paths

Before your first question, classify the request and say the classification out loud — "this looks
bounded, so I'll present a short design rather than write a spec" — so it can be overridden:

- **Spike** — a feasibility question ("can we...", "is it possible...", "quick and dirty is fine")
  whose output is an answer, not code you keep. Present the question and what you'll try in 2-3
  sentences, get a nod, then find out as cheaply as correctness allows. No design doc, no spec
  file. Report a recommendation; anything built stays labeled throwaway.
- **Bounded** — a well-scoped change to code that already exists here: a new flag, a small
  endpoint, a one-file fix. Knowing the kind of app is not enough — bounded means the flow you are
  changing is already here to read; no existing flow, not bounded. Ask the clarifying questions
  that matter, present a short design IN CHAT (a few sentences to a few short paragraphs), and
  STOP. Implementation starts only after your partner says yes — a bounded task's approval is as
  hard a gate as an architectural one. No spec file, no plan document.
- **Architectural** — new projects, new subsystems, changes that restructure how components fit
  together or alter interfaces others depend on. Full process: questions, approaches, sectioned
  design, spec, then writing-plans.

In doubt, take the heavier path. One-way ratchet: hidden complexity found mid-task upgrades it — stop, say so, step up; never downgrade.

## Checklist

Classify, announce the path, do the items in order.

**Spike:**
1. **Explore project context** — enough to frame the probe
2. **Present question + probe plan** — 2-3 sentences
3. **Get approval** — a nod is enough
4. **Investigate** — as cheaply as correctness allows
5. **Report findings** — a recommendation; label anything built throwaway

**Bounded:**
1. **Explore project context** — check files, docs, recent commits
2. **Ask clarifying questions** — one at a time, the ones that matter; why first, then write back your understanding
3. **Present short design in chat** — approach, files touched, testing
4. **Get approval** — STOP and wait for an explicit yes; presenting and starting in one breath skips the gate
5. **Implement** — the normal development workflow (TDD applies); no plan document

**Architectural:**
1. **Explore project context** — files, docs, recent commits
2. **Shared understanding first** — ask why, write back what they said vs your assumptions, wait for correction; the corrected text is the Brief's Problem
3. **Offer the visual companion just-in-time** — NOT upfront; only the first time a question is clearer shown than described, never if none arises
4. **Ask clarifying questions** — one at a time: purpose, constraints, success criteria
5. **Research check** — `references/research-checklist.md`; one item true → `Skill(znf:research)` on the scoped question; none → skip, no prior-art for show
6. **Route the design** — `references/architect-gate.md`: four features → `select-route architect`; `none` → write the memo yourself; a model → `Agent(architect)` once, then read its memo
7. **Design memo** — `references/architect-memo.md`, ten sections (2-3 approaches, gates, tokens, checklist, recommendation). Think hard before responding.
8. **Present design** — sections scaled to their complexity, user approval after each
9. **Clarify-lite** — scan the 8 Brief fields (Clear/Partial/Missing), ≤5 MC questions, log `## Clarifications`
10. **Write design doc** — save to `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md` and commit
11. **Spec self-review** — inline check for placeholders, contradictions, ambiguity, scope (below)
12. **User reviews written spec** — ask the user to review the spec file before proceeding
13. **Transition to implementation** — invoke writing-plans to create the implementation plan

## Process flow

Terminal states are path-bound: architectural ends in writing-plans only, bounded in the normal workflow, spike in a recommendation.

## The dialogue

For the bounded and architectural paths (`references/design-dialogue.md`): read `.claude/GLOSSARY.md`
for unclear domain terms, decompose oversized requests first, one question per message, propose 2-3
approaches with a recommendation, YAGNI ruthlessly, present the design in sections with a check each.

## After the Design (architectural path)

**Clarify-lite (before writing the spec):** mark each of the eight Brief fields Clear / Partial /
Missing. For Partial or Missing ones, ask at most five multiple-choice questions, one per message,
each with a one-line "why it matters". Log the answers under a dated `## Clarifications` section in
the spec — a light pass inside the dialogue, not a separate step.

**Documentation:**

- Write the spec to `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md` (user preferences for
  spec location override this default)
- Author the spec against `znf:_shared/spec-template` and follow `znf:_shared/constitution`
  (Brief-first, stable FR/SC IDs, testable SC, mark unknowns, necessity ladder, safety floor,
  traceability). Format and language rules stay in `znf:_shared/artifact-style`.
- The memo is the source of the Brief's Approach, Blast-radius, Flow, Rollback and Non-goals;
  link it from the spec header. Tier-3 route and `changed_decision` recorded per
  `references/architect-gate.md`.

**Spec Self-Review:** scan for placeholders, contradictions, over-broad scope, ambiguity; fix inline, no re-review (`references/spec-self-review.md`).

**User Review Gate:** once the review loop passes, ask the user to review it:

> "Spec written and committed to `<path>`. Please review it and let me know if you want to make any changes before we start writing out the implementation plan."

Wait. On changes, make them and re-run the review loop. Proceed only on approval.

**Implementation:** invoke writing-plans; no other skill.

## Visual Companion

A browser companion for mockups and diagrams, offered just-in-time as its own message
(`references/visual-companion-rules.md`, then `visual-companion.md`).

## References

Read one when its trigger fires.

- `references/process-flow.md` — read when you need the whole path graph.
- `references/design-dialogue.md` — read when running the dialogue itself.
- `references/red-flags.md` — read when calling a task simple, or starting before approval.
- `references/visual-companion-rules.md` — read when deciding if a question belongs in the browser.
- `references/spec-self-review.md` — read when the spec needs its fresh-eyes pass.
- `references/research-checklist.md` — read at the Research check; one true item calls `znf:research`.
- `references/architect-gate.md` — read at step 5 of the architectural path; the four features and the dispatch.
- `references/architect-memo.md` — read at step 6; the ten memo sections, shared by tier 2 and tier 3.
