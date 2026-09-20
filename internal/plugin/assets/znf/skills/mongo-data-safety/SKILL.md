---
name: mongo-data-safety
description: Use when reading or writing MongoDB documents in a multi-tenant, schemaless codebase — covers the tenant filter every query must carry, strict:false silent-write drift, raw-driver bypass, distinct-before-branch, and plural/silent-create collection traps.
allowed-tools: Read Grep Glob Bash
---

# Mongo data safety

## Tenant filter is mandatory (no row-level security)

MongoDB has no row-level security. Every query and aggregation stage that touches
a shared, multi-tenant collection must carry an explicit tenant/organization filter
in application code — the database will not add it for you, and a query missing it
returns whatever exists, not an error.

The dangerous shape is a query that is correct on a single-tenant dev database and
silently wrong once a second tenant's data lands in the same collection:

```js
// Incorrect — works in dev (only one tenant's rows exist), leaks other tenants in prod
const orders = await Order.find({ status: 'open' });

// Correct — tenant scope is part of every read and write, not an afterthought
const orders = await Order.find({ tenant_id: user.tenant_id, status: 'open' });
```

Before writing a query against a shared collection, grep for how sibling queries in
the same file/service already scope by tenant, and match that pattern rather than
inventing a new one. If no sibling query scopes it, that is a signal to ask, not to
assume scoping is unnecessary.

## strict:false writes fields silently

A schema declared with `strict: false` accepts and persists any field on write —
unknown keys are not rejected, they are written next to the ones the schema does
declare. This means "the schema doesn't declare that field" says nothing about
whether real documents carry it; only the data itself can answer that.

```js
// Schema (illustrative — mirrors a real strict:false chat/room-style schema)
const roomSchema = new Schema(
  { tenant_id: String },
  { strict: false, timestamps: { createdAt: 'created_at', updatedAt: 'updated_at' } },
);
```

```js
// Incorrect — assumes the schema is the full picture of what a document can hold
if (room.someField) { ... } // "someField" isn't in the schema, so this looks safe to skip checking

// Correct — verify against a real document before branching on a field
// db.rooms.findOne({ _id: ObjectId('...') })
```

A second, sharper version of the same trap: code that talks to the collection
through the raw driver instead of the model bypasses the schema entirely — no
validation, no defaults, nothing:

```js
// Bypasses the ODM/schema layer completely — writes/reads are unchecked
const Col = mongoose.connection.collection('raw_collection_name');
await Col.updateOne({ _id }, { $set: { any_field: value } });
```

Treat every raw `.collection(...)` call site as a place where the schema cannot
protect you — read the actual document shape before trusting a field is present
or of a given type.

## Distinct before you branch on a value

A field's declared type or enum in the schema is what *new* writes are expected to
follow — it is not a guarantee about what every existing document already holds.
Old rows, imports, and other services writing to the same collection can leave
values the current code never anticipated. Before writing a `switch`/`if` chain
that branches on a field's value, run `distinct()` on that field against the real
collection to see every value actually in use:

```js
// See every value a field really holds before branching on it
const statuses = await Order.distinct('status', { tenant_id });
const ids = await Order.distinct('customer_id', { active: true });
```

Code that branches on an assumed closed set of values (e.g. three known statuses)
will silently mis-handle the fourth value some other writer already put in the
collection — it won't throw, it will just fall through your default case wrong.

## Collection names: list, never type from memory

The collection name on disk is frequently not the singular, human-readable name
that shows up throughout the code and docs — those are commonly the name of the
model/class, not the collection. Never type a collection name from memory or copy
it from a comment; list it from the database first:

```
db.getCollectionNames()          // or the project's read-only DB helper, if one exists
db.<collection>.findOne()        // confirm shape before relying on any field
```

The reason this matters more than it looks: MongoDB creates a collection silently
on first write. Querying a mistyped or wrong-cased collection name returns zero
results with no error; writing to it creates a new, empty collection next to the
real one. A database can accumulate several empty, misnamed collections this way,
each one looking like a legitimate name to anyone who didn't verify it — and each
one a trap for the next person who copies it.

## Cross-refs

- Before changing the shape of a collection shared across services → `znf:gate`.
- Verify the real code path runs before reporting done → `znf:ship`.
