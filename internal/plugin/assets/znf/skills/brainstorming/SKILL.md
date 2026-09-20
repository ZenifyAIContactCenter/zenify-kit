---
name: brainstorming
description: "You MUST use this before any creative work - creating features, building components, adding functionality, or modifying behavior. Explores user intent, requirements and design before implementation."
---
<!-- Vendored from obra/superpowers (MIT). Adapted for the znf plugin. -->

# Brainstorming Ideas Into Designs

Turn ideas into designs and specs through collaborative dialogue. Classify how much process the
request needs, then work your path: context, refine the idea, present a design, get your human
partner's approval.

<HARD-GATE>
Do NOT invoke any implementation skill, write any code, scaffold any project, or take any
implementation action until you have told your human partner what you intend and they have approved
it. This applies to EVERY task on EVERY path below — the ceremony scales with the task; the
approval gate never does.
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

Classify first, announce the path, then do the items in order.

**Spike:**
1. **Explore project context** — enough to frame the probe
2. **Present question + probe plan** — 2-3 sentences
3. **Get approval** — a nod is enough
4. **Investigate** — as cheaply as correctness allows
5. **Report findings** — a recommendation; label anything built throwaway

**Bounded:**
1. **Explore project context** — check files, docs, recent commits
2. **Ask clarifying questions** — one at a time, the ones that matter
3. **Present short design in chat** — approach, files touched, testing
4. **Get approval** — STOP and wait for an explicit yes; presenting and starting in one breath skips the gate
5. **Implement** — the normal development workflow (TDD applies); no plan document

**Architectural:**
1. **Explore project context** — files, docs, recent commits
2. **Offer the visual companion just-in-time** — NOT upfront, only the first time a question is genuinely clearer shown than described, never if none arises. See the Visual Companion section.
3. **Ask clarifying questions** — one at a time: purpose, constraints, success criteria
4. **Research check** — `references/research-checklist.md`; one item true → `Skill(znf:research)` on the scoped question; none → skip, no prior-art for show
5. **Propose 2-3 approaches** — with trade-offs and your recommendation
6. **Present design** — sections scaled to their complexity, user approval after each
7. **Clarify-lite** — scan the 7 Brief fields (Clear/Partial/Missing), ≤5 MC questions, log `## Clarifications`
8. **Write design doc** — save to `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md` and commit
9. **Spec self-review** — inline check for placeholders, contradictions, ambiguity, scope (below)
10. **User reviews written spec** — ask the user to review the spec file before proceeding
11. **Transition to implementation** — invoke writing-plans to create the implementation plan

## Process flow

**Terminal states are path-bound.** Architectural: the ONLY skill you invoke after brainstorming is
writing-plans (research runs inside brainstorming, before the design). Bounded: the normal
development workflow, no plan document. Spike: a recommendation.

## The dialogue

This serves the bounded and architectural paths (a spike stops at "present the probe, get a nod"):
explore project context first, read `.claude/GLOSSARY.md` when a domain term is unclear, flag a
request that needs decomposing before refining details, ask one question per message, propose 2-3
approaches with a recommendation, YAGNI ruthlessly, present the design in sections with a check
after each (`references/design-dialogue.md`).

## After the Design (architectural path)

**Clarify-lite (before writing the spec):** mark each of the seven Brief fields Clear / Partial /
Missing. For Partial or Missing ones, ask at most five multiple-choice questions, one per message,
each with a one-line "why it matters". Log the answers under a dated `## Clarifications` section in
the spec — a light pass inside the dialogue, not a separate step.

**Documentation:**

- Write the spec to `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md` (user preferences for
  spec location override this default)
- Author the spec against `znf:_shared/spec-template` and follow `znf:_shared/constitution`
  (Brief-first, stable FR/SC IDs, testable SC, mark unknowns, necessity ladder, safety floor,
  traceability). Format and language rules stay in `znf:_shared/artifact-style`.

**Spec Self-Review:** with fresh eyes, scan the spec for placeholders, contradictions, over-broad scope and ambiguous requirements; fix each inline, no re-review (`references/spec-self-review.md`).

**User Review Gate:** once the review loop passes, ask the user to review it:

> "Spec written and committed to `<path>`. Please review it and let me know if you want to make any changes before we start writing out the implementation plan."

Wait. If they request changes, make them and re-run the review loop. Proceed only on approval.

**Implementation:** invoke writing-plans to create the implementation plan. Do NOT invoke any other
skill — writing-plans is the next step.

## Visual Companion

A browser companion for mockups, diagrams and visual options — a tool, not a mode. **Offer it
just-in-time, never upfront**, the first time a question is clearer shown than told. **The offer
MUST be its own message**, with nothing else in it:
> "This next part might be easier if I show you — I can put together mockups, diagrams, and comparisons in a browser tab as we go. It's still new and can be token-intensive. Want me to? I'll open it for you."

Wait for the answer. On yes, start the server with `--open`; on no, continue text-only and do not
offer again. **Then decide per question:** browser for content that IS visual (mockups, layout
comparisons, diagrams), terminal for text (requirements, trade-offs, scope). A UI *topic* is not a
visual question (`references/visual-companion-rules.md`).

If they agree to the companion, read the detailed guide first:
`skills/brainstorming/visual-companion.md`

## References

Read one when its trigger fires.

- `references/process-flow.md` — read when you need the whole path graph.
- `references/design-dialogue.md` — read when running the dialogue itself.
- `references/red-flags.md` — read when calling a task simple, or starting before approval.
- `references/visual-companion-rules.md` — read when deciding if a question belongs in the browser.
- `references/spec-self-review.md` — read when the spec needs its fresh-eyes pass.
- `references/research-checklist.md` — read at the Research check; one true item calls `znf:research`.
