---
name: explain-plan
description: Use when a diff adds or changes a DB query — the mandatory two-tier DB-perf gate. Runs a static scan (no DB needed) plus a per-query explain plan, and classifies findings BLOCKING (surface as must-fix at ship) vs ADVISORY. Degrades cleanly when the DB is unreachable.
allowed-tools: Read Grep Bash(db_read *) Bash(zenify db-perf *) Bash(git diff *)
---

# znf:explain-plan — two-tier DB-perf gate

**Announce:** "Using znf:explain-plan to run the two-tier DB-perf gate on this diff."

A query missing a usable index, an unbounded list, a missing tenant filter, or a deep skip is
**invisible on dev data** and only bites at production volume. This gate combines a static scan
(text-detectable anti-patterns, no DB) with a dynamic explain (COLLSCAN / scan-ratio / SORT), and
classifies each finding **BLOCKING** or **ADVISORY**. Static findings run even with no DB.

## Step 1 — static layer (always runs, no DB)

```bash
zenify db-perf --json           # scans origin/staging..HEAD by default
```

Read the JSON `findings[]`: each has `tier` (BLOCKING/ADVISORY/WAIVED), `signal`, `file`, `line`,
`collection`, `hint`. If `sites_scanned == 0`, write "no query in this diff", print the named line,
and stop cleanly.

## Step 2 — dynamic layer (only if the DB is reachable)

```bash
command -v db_read >/dev/null || echo "dynamic layer skipped: no db_read on PATH"
```

For each query call-site, identify the **real** collection (list it from the DB, never guess) and
run the plan. If `db_read` is missing or times out, print **"dynamic layer skipped: DB unreachable"**
and keep only the static findings — never fail.

```bash
db_read eval 'db.getCollection("<real-name>").find({…}).explain("executionStats")'
db_read sql  'EXPLAIN ANALYZE <real-statement>'
```

## Step 3 — dynamic rubric (two-tier)

Read `executionStats`: `stage`, `nReturned`, `totalDocsExamined`, `totalKeysExamined`.

| Plan shows | Tier |
|---|---|
| `IXSCAN`, ratio `totalDocsExamined/nReturned` ≤ 10 | ok |
| `COLLSCAN` on a large collection (`db_read count` > 100k) | BLOCKING |
| `Seq Scan` (SQL) on a large table | BLOCKING |
| scan-ratio > 100 | BLOCKING |
| scan-ratio 10–100 | ADVISORY |
| `SORT` stage present despite `IXSCAN` (ESR violation) | ADVISORY |
| `$lookup` whose foreign collection shows `COLLSCAN` | ADVISORY |

**An index existing ≠ an index being used.** A COLLSCAN can still happen with an index present
(wrong compound order, `$in`/`$or`, non-selective field, `$ne`/`$nin`). Read the actual `IXSCAN`
in the plan; do not infer it.

## Step 4 — merged two-tier verdict

Print one block `## DB-Perf` merging static + dynamic findings, ordered BLOCKING first. Open with
exactly one of:

- "Advisory — no BLOCKING finding." (only advisory/waived), or
- "BLOCKING — <N> finding(s) must be fixed or waived before ship."

One line per finding: `<tier> · <signal> · <file:line> · <collection> · <hint>`. A WAIVED finding
prints its logged reason. This block is what `ship` reads to decide whether to block completion; a
BLOCKING finding that is not waived means ship does not complete.
