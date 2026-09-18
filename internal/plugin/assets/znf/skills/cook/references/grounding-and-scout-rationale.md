<!-- Moved verbatim from cook/SKILL.md § Step 1: Ground the request, § Step 3: Ground the spec,
§ Step 4: /scout (W4 slim-skills). Read when: Step 1/3/4 feel redundant. -->

### From § What `/cook` does — grounding is incremental

Each grounding pass is incremental: `/ground` only fetches what has not been verified this session,
so a pass over which nothing new appeared costs nothing.

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

### From § Step 4 — brief is mixed, unlike a fix (extra, tier two)

A fix's brief is not mixed the same way: the change is entirely inside code that already has
callers, so the whole brief is the second part. A feature's brief splits, which is why the scout
gets pointed at only the half that plugs into existing consumers.

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

### The six grounding categories, spelled out (moved from § Step 3, token diet 2026-09-18)

The body names them in one line; this is what each one means.

- **DB collections/tables and fields** — list the real names from the database, never type one
  from memory.
- **API endpoints** — their request *and* response shapes.
- **Queue/event names** and the payload fields riding on them.
- **Library methods and their signatures** — read the installed types, not memory.
- **In-repo code the plan calls into** — signatures, exported symbols, component props, config
  keys. Read the definition, not a call site.
- **Env var names** — off the running process, not off `.env`.

### The scout brief, spelled out (moved from § Step 4, token diet 2026-09-18)

A feature brief is **mixed**: part is new code nothing calls yet, part plugs into code that
already has consumers. Point the scout at the second part.

1. **Who reads / writes / calls** the shared things the spec touches — in a polyrepo workspace,
   delegate that sweep to `/gate` rather than re-deriving it.
2. **Which tests cover** the code the plan will modify.
3. **What else is written in the same operation** — a queue job, a cache entry, a search index.
4. **Why the existing code is the way it is**, for anything changed rather than added.

Do not launder a partial map into a clean one: if the report says "cannot enumerate by grep",
the word "partial" travels into the plan.
