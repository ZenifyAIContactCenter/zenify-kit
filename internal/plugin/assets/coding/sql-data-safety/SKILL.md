---
name: sql-data-safety
description: Use when reading or writing SQL through a pooled driver or a thin query client (raw parameterized SQL, not a full ORM mapping) — covers the tenant filter every statement must carry, parameterization against injection, connection-pool and timeout limits, transaction scoping, and schema changes via reviewed migrations.
allowed-tools: Read Grep Glob Bash
---

# SQL data safety

For code that reaches a relational database through a **connection pool and hand-written SQL**
— a raw driver, or an ORM's data-source used only as a query client — rather than through
mapped entities and generated queries. The ORM will not scope, escape, or bound anything it is
not asked to; these are the invariants the application layer has to hold itself.

## Tenant filter is mandatory in the SQL itself

A shared multi-tenant table has no row-level security by default. Every `SELECT`, `UPDATE`, and
`DELETE` against one must carry the tenant/organization predicate **in the statement**, not in a
check performed after the rows come back.

The dangerous shape is the statement that is correct on a single-tenant dev database and silently
wrong once a second tenant's rows share the table:

```js
// Incorrect — the row is fetched, then the tenant is checked in app code.
// The DELETE is keyed only on the natural id, so a concurrent change,
// a refactor that drops the pre-check, or a non-unique id deletes another tenant's row.
const row = await db.query('SELECT * FROM page WHERE page_id = ?', [pageId]);
if (row?.tenant_id !== tenantId) throw new Forbidden();
await db.query('DELETE FROM page WHERE page_id = ?', [pageId]);

// Correct — the tenant predicate is part of the write. It cannot be lost,
// and it cannot race a check that ran a moment earlier.
await db.query('DELETE FROM page WHERE page_id = ? AND tenant_id = ?', [pageId, tenantId]);
```

An app-level check guards the read you happened to write; the predicate in the statement guards
the row. Put it in every statement, including the write path, and do not rely on a natural key
being globally unique.

## Parameterize every value — never interpolate

Every value that varies at runtime goes through a placeholder (`?` or a named parameter), never
string concatenation or a template literal. This is the whole of SQL-injection defense in
hand-written SQL; there is no second layer under it.

```js
// Incorrect — any input containing quotes or ';' rewrites the query.
db.query(`SELECT * FROM users WHERE email = '${email}'`);

// Correct — the value can never change the statement's structure.
db.query('SELECT * FROM users WHERE email = ?', [email]);
```

Identifiers (table and column names) cannot be parameterized by most drivers. When one must be
dynamic, select it from a fixed allow-list in code — never pass it through from a request.

A dynamic `WHERE` built from optional filters is the usual place interpolation creeps back in:
build the clause as a list of `field = ?` fragments joined with `AND`, pushing each value onto a
params array in lock-step, so the SQL text stays constant per filter and every value is still
bound.

## Bound the connection pool and its timeouts

One pool per process, created once at startup and reused — never a connection or a data-source
per request (that exhausts the database's connection slots under load). Set an explicit maximum
pool size; the driver default is small (often 10) and is a ceiling you should choose on purpose.

Set timeouts explicitly, because the defaults are frequently "wait forever":

- a **connect/acquire** timeout, so a saturated pool fails fast instead of hanging the request;
- a **statement/query** timeout, so one slow query cannot pin a connection indefinitely;
- keep-alive on the socket if the driver offers it, so idle pooled connections are not silently
  dropped by an intermediary and then reused dead.

Decide deliberately what happens when the database is unreachable at startup: **fail-closed**
(refuse to start) for a service that cannot function without it, or **fail-open** (start, log,
serve what it can) for one where the SQL path is optional. Both are valid; the bug is not
choosing — a silent hang while a pool never fills is the worst of the three.

## Scope transactions to what needs atomicity

Wrap a transaction only around operations that must commit or roll back together; do not
blanket-wrap unrelated work — it holds a connection and locks longer than needed.

Two things go wrong most often:

- **Using the wrong handle inside the transaction.** Every statement in a transaction must run
  through the transaction's own manager/connection. A call that reaches for the global pool
  handle instead runs *outside* the transaction and will not roll back with it.
- **Leaking the connection.** When you drive a transaction with an explicitly acquired
  connection (begin → commit/rollback), release it in a `finally` — an early return or a thrown
  error on the happy path otherwise leaks it until the pool is exhausted.

```js
const conn = await pool.getConnection();
try {
  await conn.beginTransaction();
  await conn.query('UPDATE account SET balance = balance - ? WHERE id = ? AND tenant_id = ?', [amt, from, t]);
  await conn.query('UPDATE account SET balance = balance + ? WHERE id = ? AND tenant_id = ?', [amt, to, t]);
  await conn.commit();
} catch (e) {
  await conn.rollback();
  throw e;
} finally {
  conn.release(); // always — even on the paths above
}
```

## Schema comes from reviewed migrations, never from app startup

Never let the data layer alter the schema on boot (`synchronize: true` and its equivalents) — it
can drop columns and lose data when the code's model and the live table disagree. Keep it off in
every environment.

Change schema through migrations that are: committed to version control, small and ordered,
descriptively named, and **reviewed before they run** — read the up and the down. Run them as a
deliberate, separate step (pre-deploy), not automatically on application start. A hand-written
ad-hoc `ALTER` run outside this flow leaves no record of what the schema is or how it got there.

## Read only what you need

Select explicit columns instead of `SELECT *` — projecting everything couples callers to column
order and drags large/unused columns over the wire. Paginate any list that can grow (limit +
offset, or a cursor); an unbounded list query is a latency and memory incident waiting for the
table to get big enough.

When a statement spans more than one database on the same server, qualify the table names
explicitly (`other_db.table`) rather than relying on the connection's default database — the
default is a property of how the pool was configured, not of the query, and it drifts.

## Cross-refs

- Multi-tenant document stores: see `mongo-data-safety` (same tenant-scoping invariant, no
  row-level security, expressed for a schemaless collection).
- Cross-service contracts written in the same operation as a SQL row: see `service-integration`
  (dual-write safety and idempotency).
