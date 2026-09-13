<!-- Moved verbatim from cook/SKILL.md § Step 1: Ground the request, § Step 3: Ground the spec,
§ Step 4: /scout (W4 slim-skills). Read when: Step 1/3/4 feel redundant. -->

### From § Step 1 — the `chatbot_setting` example (extra, tier two)

Enough to stop a design being built on `chatbot_setting` when the collection is
`chatbot_settings`.

### From § Step 1: Ground the request — before brainstorming

This is not an override of `brainstorming` — it is doing the data half of that skill's own
step 1, "explore project context", properly, and handing the result in. The skill is then run
exactly as written.

Why before rather than after: `brainstorming` ends with **two user gates** — approval per
design section, then approval of the written spec. Grounding only after those means the user
can approve a design resting on a collection that does not exist, and the correction costs
another round of *their* review, not yours. Cheaper to arrive with the real names.

### From § Step 3: Ground the spec — before the plan, not after

This runs **before** `writing-plans` on purpose: that skill forbids placeholders and demands
real code in every task, so a wrong collection or field name gets written *into the plan*, and
SDD then hands implementers the brief with "the exact values to use verbatim". Grounding after
the plan grounds a contaminated requirement. This is the path that put three empty junk
collections into a shared production database.

Ground inline. There is no DB delegate — the `db-schema-fetcher` agent was deleted after 0
dispatches in 1427 transcripts, and `/ground` explains why inline is the right place: what this
step produces is the shape the code gets written from, so a summary of it is not a substitute.

### From § Step 4 — why scout, and why dispatched early (extra, tier two)

That decision needs to know what already exists and what depends on it. Both steps take the
approved spec as their only input, so nothing about grounding informs the scout brief. That
overlap is free: it needs no extra agent and changes no output, only the order the two are
started in.

### From § Step 4: `/scout` — what depends on what the spec is about to change

The numbering stays 3 → 4 because that is the order the *results* are consumed; only the dispatch moves
earlier.

Grounding cannot answer this. It is forward-only — it tells you a name is real, never whether
changing it breaks a caller you did not know about. Opposite directions, and the second one is
where existing behaviour gets broken.
