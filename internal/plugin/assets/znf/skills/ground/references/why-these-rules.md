# Ground — why each rule exists

Evidence and history behind the rules in `SKILL.md`.

## Why no delegate agent, and the db-schema-fetcher history

What this skill produces *is* the shape the code is then written from. Route it through a summary
and you write from a **summary** of a shape instead of the shape — the exact second-hand step this
skill exists to remove. `/ship` states the same rule for its lint output: a summary is not the
output, and the step exists to look at the output.

There used to be a `db-schema-fetcher` agent offered here as an optional delegate for a heavy
fetch. It was deleted after **0 dispatches across 1427 session transcripts**. The wording was the
reason — *"you **may** delegate"* is an invitation, and an invitation is not a mechanism, which is
the failure shape that has recurred throughout this kit. But the outcome was also correct: fetching
the shape where it will be used is the right place, so the honest fix was to remove the option
rather than add a trigger for it.

Several collections is still not a reason to split the work — it is a reason to run several
`zenify db-read` calls. They are independent, cheap, and their output is the evidence.

## Why the collection name is listed, never typed from memory

A `<collection>` placeholder in this very file got executed verbatim, and a shared production
Mongo has contained a collection literally named `collection` ever since. MongoDB creates a
collection silently on first write, so a wrong name returns zero rows with no error at all.

Do not grep a credential out of `.env` — a wrong value there once cost a whole session, and `.env`
on disk is not what the running container was started with.

## Why one document is not the schema

If the collection is multi-tenant or the keys are dynamic, `findOne()` gives you **one tenant's**
convention. Real incident: a key scheme grounded from one tenant as `ad_(str|long|date)_N` was
`addition_<slug>_<ts>` for another; the parser threw, the error was swallowed, and both the
frontend and the backend broke the same way.

Derive a field's kind from its declared `type`, never from a prefix in its key name.

## Why values matter, not only shapes

The store holds every shape every version ever wrote — it is the union of all of them, minus
whatever was migrated by hand. In a schemaless store that drift is not a risk, it is a certainty:
`strict: false` is declared **pervasively** across the backend repos, so unknown fields were
written silently, and a large share of access goes through the **raw driver**, bypassing the schema
entirely. So "the schema does not declare that field" tells you nothing about the data.

No count here on purpose. Three attempts to measure the `strict: false` files disagreed — 107,
137, 288 — differing only by whether the tool honoured `.gitignore` and whether generated caches
were excluded. The decision is identical at 100 or at 300. If you need a number, run the command
and quote it with the number:

```bash
rg -l --glob '*.{js,ts}' 'strict:\s*false' | wc -l
```

If `distinct` returns a value your new code has no branch for, that is a bug already written. One
documented case: 11% of historical orders rendering "Unknown User" for months, with nothing
failing and no error anywhere.

The same question applies to a **new required field**: it is absent from every existing document.
Adding it without a backfill means the invariant your code assumes is false for all data written
before today. Ordering that works: add it optional → switch all writers → backfill in batches →
only then require it.

## Why the tenant filter is the most severe class

In a multi-tenant store this is the most common root cause of cross-tenant leaks. `tenant_id` runs
through **essentially every collection** here, and MongoDB has **no row-level security** — so the
application layer is the only enforcement, with no database backstop underneath it. Postgres would
at least let you make the filter mandatory in the database; here nothing does.

The trap defeats ordinary testing: **a query missing its tenant filter returns correct-looking
results in development**, because dev data holds one tenant. Nothing looks wrong until production,
where it returns other customers' rows.

If the project has a repository or middleware layer that injects the filter, use it; a hand-built
query that takes the tenant as an optional parameter is the failure waiting to happen.

## Why config values are read live

Config changes cause roughly **31% of change-induced outages** against about 37% for code — nearly
as dangerous, and reviewed far less carefully. A regex in a config rule once took a global network
to ~100% CPU in under a minute.

Env vars have the same problem in the other direction: `.env` on disk and the environment a
container was actually started with drift apart, and the file will lie to you.
