---
name: ground
description: Verify real shapes and real values before writing code. Use when about to write code that touches a DB field, API payload, queue message, external library API, in-repo function/symbol/component props, a config key or an env var — fetch the ACTUAL shape from its real source first, including which values a field really holds and which filters every query must carry. Answers "what is X?" only; for "what depends on X?" use /scout.
allowed-tools: Read Grep Glob Bash(db_read *) Bash(mongosh *) Bash(mysql *) Bash(psql *) Bash(grep *) Bash(find *) Agent
---

**Rigid discipline.** This skill enforces one rule: **read before write**.

**This skill answers one direction only: "what is X?"** The reverse question — *"what depends
on X?"* — is `/scout`, and it is a different activity: discovery rather than verification,
outward rather than forward. Grounding a name perfectly protects you from using something that
does not exist. It does nothing to stop you breaking a caller you never knew about. If you are
**changing** something that already exists, you need both, and neither substitutes for the other.

## When this skill triggers

Invoke `/ground` (or let it auto-trigger) when:
- About to use a DB field name you haven't verified this session
- About to call an API endpoint or use a response field
- About to publish/consume a queue message
- About to call a library method you remember but haven't checked
- About to use an env var name
- About to call a function, import a symbol, or pass props to a component **in this repo** that you haven't read this session (house rule #1 names files and functions explicitly — this is the case that gets violated most)

## Step 1: Identify what needs grounding

State which data shapes are unverified:
- DB collections/tables and fields — **and, for any field the code will branch on, what values
  it actually holds** (see "Ground the data, not only the shape" below)
- **Filters that every query here must carry** — a shape question with the highest cost when
  wrong (see below)
- API endpoints and response shapes
- Queue/event names and payload fields
- Library methods and their signatures
- In-repo code you will call into: function/method signatures, exported symbol names, component props, config keys
- Env var names — **and config values**, read live, not from a file

## Step 2: Fetch the real shape

**DB — resolve the NAME before you query it.** Never type a collection or table name
from memory, and never leave a `<placeholder>` in a command for yourself to substitute:
a `<collection>` placeholder in this very file got executed verbatim, and a shared
production Mongo has contained a collection literally named `collection` ever since. **List the names,
then pick one.** MongoDB creates a collection silently on first write, so a wrong name
returns zero rows with no error at all.

Use the project's documented read-only accessor. It takes credentials from a designated
store, so none is extracted by hand, passed as an argument, or printed. **Its name is in the
project's `CLAUDE.md`** — `/onboard-project` step 7 is what puts it there. The commands below use
`db_read` as the placeholder name; substitute whatever this project calls it:

```bash
db_read collections <substr>              # real Mongo names — start here, never guess one
db_read doc <a-name-from-that-list>       # one real document
db_read tables <substr>                   # real MySQL table names
db_read sql 'DESCRIBE <a-real-table>'     # real MySQL columns
```

If a project has no such accessor, read its CLAUDE.md for how to reach real data and
**write one** rather than pasting a raw connection command with a placeholder in it. Do
not grep a credential out of `.env` — a wrong value there once cost a whole session, and
`.env` on disk is not what the running container was started with.

One document is not the schema. If the collection is multi-tenant or the keys are
dynamic, `findOne()` gives you **one tenant's** convention — sample a second tenant
before generalising, and derive a field's kind from its declared `type`, never from a
prefix in its key name. (Real incident: a key scheme grounded from one tenant as
`ad_(str|long|date)_N` was `addition_<slug>_<ts>` for another; the parser threw, the
error was swallowed, and both FE and BE broke the same way.)

### Ground the data, not only the shape

"The field exists" is not enough when your code **branches on its value**. The store holds
every shape every version ever wrote — it is the union of all of them, minus whatever was
migrated by hand. In a schemaless store that drift is not a risk, it is a certainty: `strict:
false` is declared **pervasively** across the backend repos, so unknown fields were written
silently, and a large share of access goes through the **raw driver**, bypassing the schema
entirely. So "the schema does not declare that field" tells you nothing about the data.

(No count here on purpose. Three attempts to measure the `strict: false` files disagreed — 107,
137, 288 — differing only by whether the tool honoured `.gitignore` and whether generated caches
were excluded. The decision is identical at 100 or at 300. If you need a number, run the command
and quote it with the number: `rg -l --glob '*.{js,ts}' 'strict:\s*false' | wc -l`.)

So for any field a condition, `switch`, or enum comparison will read:

```bash
db_read eval 'db.getCollection("<a-real-name>").distinct("<field>")'   # every value that exists
db_read count <a-real-name>                                            # how much data you are landing on
```

If `distinct` returns a value your new code has no branch for, that is a bug already written.
One documented case of exactly this: 11% of historical orders rendering "Unknown User" for
months, with nothing failing and no error anywhere.

The same question applies to a **new required field**: it is absent from every existing
document. Adding it without a backfill means the invariant your code assumes is false for all
data written before today. Ordering that works: add it optional → switch all writers → backfill
in batches → only then require it.

### Ground the filters a query must carry

Not "which fields exist" but **"what does a correct query here always include"**. In a
multi-tenant store this is the single most severe class of mistake and the most common root
cause of cross-tenant leaks. `tenant_id` runs through **essentially every collection** here, and
MongoDB has **no row-level security** — so the application layer is the only enforcement, with no
database backstop underneath it. Postgres would at least let you make the filter mandatory in the
database; here nothing does.

The trap is specific and it defeats ordinary testing: **a query missing its tenant filter
returns correct-looking results in development**, because dev data holds one tenant. Nothing
looks wrong until production, where it returns other customers' rows.

So before writing a query, read how the existing queries against that collection are written —
which predicate appears in every one of them — and carry it. If the project has a repository or
middleware layer that injects the filter, use it; a hand-built query that takes the tenant as an
optional parameter is the failure waiting to happen.

### Config values, not just config key names

Treat config with the same suspicion as env vars: **read the value that is live**, not the key
name and not the file. Config changes cause roughly **31% of change-induced outages** against
about 37% for code — nearly as dangerous, and reviewed far less carefully. A regex in a config
rule once took a global network to ~100% CPU in under a minute.

**API:** Grep for the handler/route that writes the response:
```bash
grep -rn 'res.json\|response.json\|return {' src/routes/ | grep <endpoint>
```

**Queue/Event:** Grep for the publisher:
```bash
grep -rn 'emit\|publish\|produce' src/ | grep '<event_name>'
```

**Library:** Read the installed types:
```bash
find node_modules/<lib> -name "index.d.ts" | head -1 | xargs head -100
```

**In-repo code:** Read the definition, not the call sites — a call site shows how someone
else used it, not what it accepts. Locate it, then Read the actual signature/body:
```bash
grep -rn 'function <name>\|const <name>\|class <name>\|export .*<name>' src/
```

**Env vars:** read them off the **running process**, not a file — `.env` on disk and the
environment a container was actually started with drift apart, and the file will lie to you:
```bash
docker exec <container> printenv | grep <VAR>   # or printenv in the running shell
```

## Step 3: Confirm and proceed

After fetching, state the VERIFIED field names/shape. Only then proceed to write code using them.

**Do this inline. There is no delegate, and that is deliberate.**

What this skill produces *is* the shape the code is then written from. Route it through an agent and
you write from the agent's **summary** of a shape instead of the shape — the exact second-hand step
this skill exists to remove. `/ship` states the same rule for its lint output: an agent hands back its
summary, and the step exists to look at the output.

There used to be a `db-schema-fetcher` agent offered here as an optional delegate for a heavy fetch.
It was deleted after **0 dispatches across 1427 session transcripts**. The wording was the reason —
*"you **may** delegate"* is an invitation, and an invitation is not a mechanism, which is the failure
shape that has recurred throughout this kit. But the outcome was also correct: inline is the right
place, so the honest fix was to remove the option rather than add a trigger for it.

Several collections is still not a reason to delegate — it is a reason to run several `db_read` calls.
They are independent, cheap, and their output is the evidence.

## Red-flag table (stop if any apply)

| Flag | Action |
|---|---|
| "I remember this field" | Re-fetch; memory is stale |
| "The name is obvious" | Dynamic code doesn't enforce obvious; fetch anyway |
| No .env or DB config visible | Ask user for connection before proceeding |
| Library version differs from training | Read installed types; don't use training memory |
| A doc, README, or CLAUDE.md states the shape | Documentation is not ground truth — it goes stale silently. Verify against live data, the running process, or the code definition. Ground truth is what executes, not what describes |
| Verified it in one tenant / one environment | One sample is not the shape — check a second before generalising |
| "The field exists, that's enough" | Not if the code branches on its value. `distinct()` it — the store holds every shape ever written |
| "The query returns the right rows locally" | Dev data has one tenant, so a missing tenant filter looks correct. Check the predicate, not the result |
| "I grounded it, so the change is safe" | Grounding is forward-only. Whether you break an existing caller is `/scout`'s question, and it is unanswered until you ask it |
