---
name: ground
description: Verify real shapes and real values before writing code. Use when about to write code that touches a DB field, API payload, queue message, external library API, in-repo function/symbol/component props, a config key or an env var — fetch the ACTUAL shape from its real source first, including which values a field really holds and which filters every query must carry. Answers "what is X?" only; for "what depends on X?" use /scout.
argument-hint: "<names / shapes to verify>"
allowed-tools: Read Grep Glob Bash(zenify db-read *) Bash(mongosh *) Bash(mysql *) Bash(psql *) Bash(grep *) Bash(find *) Agent
context: fork
background: false
---

**The things to ground are given in `$ARGUMENTS`.** Verify each one against its real source and
report back. You have no other context — do not assume anything about the caller's plan.

Tool names for each action: `znf:_shared/harness-tools` (harness mapping table).

**Rigid discipline.** This skill enforces one rule: **read before write**.

**It answers one direction only: "what is X?"** The reverse — *"what depends on X?"* — is
`/scout`. Grounding a name protects against using something that does not exist; it does nothing
to stop an existing caller being broken.

## Step 1: Identify what needs grounding

From `$ARGUMENTS`, state which shapes are unverified:
- DB collections/tables and fields — **and, for any field the code will branch on, what values
  it actually holds**
- **Filters that every query here must carry** (see below)
- API endpoints and response shapes
- Queue/event names and payload fields
- Library methods and their signatures
- In-repo code: function/method signatures, exported symbol names, component props, config keys
- Env var names — **and config values**, read live, not from a file

If the workspace has `.claude/GLOSSARY.md`, read it when a domain term is unclear — a term you
cannot define is a shape you cannot ground.

## Step 2: Fetch the real shape

**DB — resolve the NAME before you query it.** Never type a collection or table name from memory,
and never leave a `<placeholder>` in a command for yourself to substitute. **List the names, then
pick one.** A wrong Mongo name returns zero rows with no error (see references).

Use the project's documented read-only accessor. It takes credentials from a designated store, so
none is extracted by hand, passed as an argument, or printed. The kit ships `zenify db-read`; if
the project's `CLAUDE.md` names a different accessor, use that name:

```bash
zenify db-read collections <substr>              # real Mongo names — start here, never guess one
zenify db-read doc <a-name-from-that-list>       # one real document
zenify db-read tables <substr>                   # real MySQL table names
zenify db-read sql 'DESCRIBE <a-real-table>'     # real MySQL columns
```

If a project has no such accessor, read its CLAUDE.md for how to reach real data and **write
one** rather than pasting a raw connection command with a placeholder in it.

**One document is not the schema.** If the collection is multi-tenant or the keys are dynamic,
sample a second tenant before generalising, and derive a field's kind from its declared `type`,
never from a prefix in its key name.

### Ground the data, not only the shape

"The field exists" is not enough when the code **branches on its value**. The store holds every
shape every version ever wrote. So for any field a condition, `switch`, or enum comparison reads:

```bash
zenify db-read eval 'db.getCollection("<a-real-name>").distinct("<field>")'   # every value that exists
zenify db-read count <a-real-name>                                            # how much data you are landing on
```

If `distinct` returns a value the new code has no branch for, that is a bug already written. A
**new required field** is absent from every existing document: add it optional → switch all
writers → backfill in batches → only then require it.

### Ground the filters a query must carry

Not "which fields exist" but **"what does a correct query here always include"**. In a
multi-tenant store this is the most severe class of mistake: a query missing its tenant filter
**returns correct-looking results in development**, because dev data holds one tenant.

Before writing a query, read how the existing queries against that collection are written — which
predicate appears in every one of them — and carry it.

### Ground the query plan (shift-left)

Grounding a query is the cheapest moment to see its plan. This is the **mandatory** DB-perf gate:
when the change adds or touches a backend DB query, delegate to **`Skill(znf:explain-plan)`** — it
runs `zenify db-perf` (the static two-tier scan, no DB needed) plus the dynamic explain, reading
each query's plan (`zenify db-read eval '…explain("executionStats")'` / `EXPLAIN ANALYZE`) and
flagging a `COLLSCAN` / `Seq Scan` on a large collection before the code is even written. At ground
time the gate is advisory; the **same** gate runs with teeth at `/ship`. A missing
`Skill(znf:explain-plan)` line here when the diff touches a query means the gate was skipped.

### Config values, not just config key names

Treat config with the same suspicion as env vars: **read the value that is live**, not the key
name and not the file.

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

**Env vars:** read them off the **running process**, not a file:
```bash
docker exec <container> printenv | grep <VAR>   # or printenv in the running shell
```

## Step 3: Report the verified shapes VERBATIM

**Return the exact text, never a paraphrase.** The caller writes code from this report and cannot
see what you saw. So the report carries:

- exact field / collection / table / symbol names, spelled as the source spells them
- the distinct values a branched-on field actually holds, listed in full
- exact signatures, props and config keys
- the commands you ran and the output lines that answered them
- for anything you could NOT verify: say so by name. Never fill a gap with a plausible name.

A summary of a shape is not the shape, which is the second-hand step this skill exists to remove.

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
| "I grounded it, so the change is safe" | Grounding is forward-only. Whether an existing caller breaks is `/scout`'s question, and it is unanswered until you ask it |

## References

- `references/why-these-rules.md` — the incidents behind the name-listing, one-tenant, distinct-values, tenant-filter and live-config rules, and why there is no delegate agent.
