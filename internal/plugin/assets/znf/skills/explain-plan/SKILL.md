---
name: explain-plan
description: Use when a diff adds or changes a DB query — reads each query's plan and flags a missing-index scan (COLLSCAN / Seq Scan) on a large collection before it hits production. advisory only, never blocks.
allowed-tools: Read Grep Bash(db_read *)
---

# znf:explain-plan — inspect query plans, catch COLLSCAN early

**Announce:** "Using znf:explain-plan to check query plans in this diff."

A query missing a usable index is **invisible on dev data** (a few thousand documents, a full scan
is still fast) and only bites at production volume. This skill reads the plan of every query in the
diff and reports a `COLLSCAN` (Mongo) / `Seq Scan` (SQL) on a large collection/table. **Advisory:**
reports findings, does NOT block progress. Fails open — missing `db_read` or no query means a clean
stop, no error reported.

## When to use

- When grounding or shipping a change that touches the DB (cook/ship/ground call it here).
- Or invoke by hand on any diff that adds/changes a query.

## Step 1 — mechanical trigger (from the diff)

Count query call-sites in the diff. If `db_read` isn't on PATH, this skill doesn't apply to this project.

```bash
command -v db_read >/dev/null || echo "no db_read on PATH — this skill doesn't apply in this project"
git diff HEAD | rg -c '\.find\(|\.aggregate\(|\.findOne\(|\.updateMany\(|\.skip\(|OFFSET|JOIN'
```

Non-zero → move to Step 2. Zero → write "no query in this diff" and stop cleanly.

## Step 2 — run explain per site

For each call-site: identify the **real** collection/table (don't guess — list it from the DB) and
the filter shape, then run the plan. This skill does NOT hardcode any name; every name comes from
the diff being inspected.

```bash
# Mongo
db_read eval 'db.getCollection("<real-name>").find({…}).explain("executionStats")'
# Relational (MySQL/Postgres)
db_read sql 'EXPLAIN ANALYZE <real-statement>'
```

If the filter is a variable/builder-chain (can't be built directly), read the code and reconstruct a representative value by hand.

## Step 3 — read the plan with a size-aware rubric

| Plan shows | Conclusion |
|---|---|
| `IXSCAN` / index used | ok |
| `COLLSCAN` on a LARGE collection | FINDING |
| `Seq Scan` on a LARGE table | FINDING |

Important note: **an index existing ≠ an index being used**. A full scan can still happen even with
an index present when: the compound index has the wrong column order · the shape is `$in` / `$or` ·
the field is non-selective. Read the actual `IXSCAN` in the plan — don't infer it from "this collection has an index."

## Step 4 — advisory report

Open with: "Advisory — does not block progress." One line per finding:

```
<file:line> · <collection/table> · scan verb (COLLSCAN/Seq Scan) · suggested index to add
```

No findings → state clearly that N sites were inspected, all IXSCAN. Never blocks, even with findings.
