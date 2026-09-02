---
name: mongoose-modeling
description: Use when designing or changing a Mongoose schema — schema/index design, lean vs populate, discriminators, and the safe order to add a new field (optional → switch writers → backfill → require).
allowed-tools: Read Grep Glob Bash
---

# Mongoose modeling

## Schema & index design

Every field that a query filters or sorts by needs an index — a filter without one
forces a full collection scan, which is invisible on a small dev database and slow
the moment the collection has real volume. A single-field index covers a lookup on
that field alone; a compound index covers a query that filters/sorts on several
fields together, and its field order matters — it should match the order the query
actually filters by (equality fields first, range/sort field last):

```js
// Single-field indexes on fields queried individually
const messageSchema = new Schema({
  room_id: { type: String, required: true, index: true },
  tenant_id: { type: String, required: true, index: true },
  status: { type: String, required: true, index: true },
});

// Compound index matching the real query shape: filter by tenant+room, sort by time
messageSchema.index({ tenant_id: 1, room_id: 1, created_at: -1 });
```

A `unique: true` compound index is also how a "one row per combination" invariant
gets enforced at the database layer instead of only in application code:

```js
// One config row per (tenant, brand, source) combination — DB rejects duplicates
sourceSchema.index(
  { tenant_id: 1, brand_id: 1, source: 1 },
  { unique: true },
);
```

Before adding an index to a collection with existing data, check whether the ODM
builds indexes automatically on connect (`autoIndex`) — many production setups
disable it, so a schema-level index declaration alone does not guarantee the index
exists in the real database. Confirm on the live collection, and if it must be
added to already-populated data, write it as a migration that counts conflicting
duplicates before attempting a unique index, rather than letting `createIndex`
fail midway.

## `lean()` vs `populate()`

A normal Mongoose query returns full documents — hydrated instances with getters,
virtuals, and change tracking. `.lean()` skips all of that and returns plain
JavaScript objects, which is faster and lighter for read paths that only serialize
the result (an API response, a report row) and never call a document method or
save it back:

```js
// Read-only path: skip hydration entirely
const rooms = await Room.find({ tenant_id }).lean();

// Path that will call an instance method or re-save the document: keep it hydrated
const room = await Room.findById(id);
room.markAsRead();
await room.save();
```

`.populate()` replaces a reference field with the referenced document via a
second query (or several, for nested populate) — convenient, but each populated
path is an extra round trip. On a list endpoint, populating a field only to read
one property off it is usually cheaper as a single `$lookup` aggregation stage, or
as a targeted `.select()` on the populate call:

```js
// Populates the whole referenced document
const tickets = await Ticket.find({ tenant_id }).populate('assignee_id');

// Cheaper: only pull the fields actually used downstream
const tickets = await Ticket.find({ tenant_id })
  .populate('assignee_id', 'name email')
  .lean();
```

`.lean()` and `.populate()` compose fine together — lean only affects hydration of
the top-level result, not whether references get resolved.

## Discriminator pattern

A discriminator lets several document shapes share one collection while each
subtype keeps its own fields, instead of splitting them into separate collections
or overloading one schema with fields that are only meaningful for some rows. Use
it when the shapes share a real base schema (common fields, common indexes,
queried together) and diverge only in a few subtype-specific fields:

```js
const eventSchema = new Schema(
  { tenant_id: String, type: String },
  { discriminatorKey: 'type' },
);
const Event = model('Event', eventSchema);

const CallEvent = Event.discriminator(
  'call',
  new Schema({ duration_seconds: Number }),
);
const ChatEvent = Event.discriminator(
  'chat',
  new Schema({ message_count: Number }),
);
```

A softer version of the same idea, without the ODM feature, is a single schema
where one or more fields act as an informal discriminator: their meaning, or
whether they are populated at all, depends on the value of another field on the
same document (e.g. a `source` enum field that decides which of several optional
fields is relevant, with the rest left `null`). This is common where a full
`.discriminator()` split would be overkill for two or three variant fields — but
it pushes the "which fields matter" logic into every reader, so document it on the
schema at the point the deciding field is declared, not only in application code.

## Adding a field safely: optional → switch writers → backfill → require

Adding a `required` field to a schema that already has documents breaks every
existing document the moment validation runs against it — reads that don't
re-save are unaffected, but any write path that loads-then-saves an old document
now fails validation on a field it never set. Roll a new field out in this order
instead of adding it as `required` from the start:

1. **Optional, no default (or a safe default).** Deploy the schema change with
   the field optional. Existing documents are untouched; nothing breaks.
2. **Switch writers to populate it.** Deploy the code that starts setting the
   field on every new write. At this point old documents still lack it, new ones
   have it.
3. **Backfill existing documents.** Run a migration that sets the field on every
   document that predates step 2, in batches, with a way to re-run it safely if
   it's interrupted partway.
4. **Make it required (optional).** Only once the backfill is confirmed complete
   — checked against the real collection, not assumed — is it safe to mark the
   field `required` and rely on it unconditionally in application code.

Skipping straight to `required`, or writing application code that assumes the
field is always present before the backfill has actually finished, is the same
failure mode as any other unverified assumption about document shape: it works on
whatever subset of data you tested against and breaks on the rest.

## Cross-refs

- Changing the shape of a collection shared across services → `znf:gate`.
- Runtime read/write traps on an already-designed schema (tenant filters,
  `strict: false`, raw-driver bypass) → the `mongo-data-safety` skill.
