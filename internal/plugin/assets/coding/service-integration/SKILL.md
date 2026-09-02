---
name: service-integration
description: Use when changing a cross-service contract — a Redis pub/sub payload, a BullMQ job (mind queue-name env split), or an HTTP shape between services. Covers publisher↔subscriber pairing, dual-write safety, idempotency, and contract versioning.
allowed-tools: Read Grep Glob Bash
---

# Service integration patterns

A polyrepo system has no compiler across repo boundaries. A Redis channel
name, a BullMQ queue name, and an HTTP response shape are contracts enforced
by nothing but convention — grep is the only thing that catches a drifted
one before it reaches runtime.

## Change a pub/sub payload — both ends

A publisher and its subscriber(s) live in different repos and agree on a
shape only by convention. Changing the payload on one side without the other
fails silently: the subscriber still receives a message, it just reads
`undefined` for a renamed or removed field, and nothing throws.

```js
// publisher
await redisClient.publish(CHANNEL_ORDERS_CREATED, JSON.stringify({
  order_id: order._id,
  tenant_id: order.tenant_id,
  total: order.total,
}));
```

```js
// subscriber, in a different repo
const handler = async raw => {
  const {order_id, tenant_id, total} = JSON.parse(raw);
  // a field renamed on the publisher side shows up here as `undefined`,
  // not as an error
};
await redisSub.subscribe(CHANNEL_ORDERS_CREATED, handler);
```

Before touching either side: grep the channel-name constant (not the literal
string — it is usually assembled from a config module) across every repo
that might subscribe, not just the one obvious consumer. A channel commonly
has more than one subscriber (a socket bridge that forwards to connected
clients, and a separate service that reacts to the same event), and each one
breaks independently if the shape changes under it. Add a field additively
first (old readers ignore an unknown key), ship both sides, then remove the
old field in a follow-up change once every subscriber has deployed.

## BullMQ job + queue-name env split

A BullMQ producer and its worker must resolve to the **same queue name** at
runtime — not just the same literal in source. A queue name is typically
read from an env var with a fallback:

```js
// shared constant, read by both the producer repo and the worker repo
QUEUE_EMAILS: process.env.QUEUE_EMAILS || 'EMAILS_DEV',
```

If the producer's deployed environment defines `QUEUE_EMAILS` but the
worker's does not (or vice versa — a partial rollout, a container restarted
with a stale env file), they silently split into two independent queues:
`new Queue('EMAILS', ...)` on one side, `new Worker('EMAILS_DEV', ...)` on
the other. The producer keeps enqueuing successfully — `add()` never errors
because nothing is listening — and jobs pile up on a queue with zero
consumers. There is no exception, no dead-letter signal, nothing in the
producer's logs; the only symptom is that the downstream effect (an email
never sent, a task never created) never happens.

```js
const worker = new Worker(queueName, jobHandler, {connection});
const queue = new Queue(queueName, {connection});
```

When adding or renaming a queue: define the name in exactly one place per
repo, have every producer and every consumer import it from that place
rather than re-deriving the fallback string locally, and when diagnosing a
job that never runs, check the queue's actual consumer count on the Redis
instance in use before assuming the handler code is broken — a queue with
the right name and zero consumers looks, from the producer's side, exactly
like a queue that is working.

## HTTP contract between services

An HTTP call from one service to another is a contract with no shared type
system backing it. The caller typically wraps the client once, with an
explicit timeout — an HTTP call with no timeout can hang a request handler
indefinitely when the callee is slow or down:

```js
class OrdersClient {
  constructor() {
    this.http = axios.create({timeout: 10000});
  }

  async fetchInvoice(invoiceId) {
    return this.http.get(`${BILLING_BASE_URL}/invoices/${invoiceId}`);
  }
}
```

Before changing a response shape a caller depends on: grep every caller of
that endpoint, not just the ones in the same repo — an internal endpoint
called from two other services breaks both if a field is renamed or an
envelope is restructured. Treat a shape change the same as a pub/sub payload
change: add fields additively, deploy the producer, then let each caller
adopt the new field before removing the old one.

## Dual-write silent failure

Writing to two systems that are not in one transaction (Mongo plus a search
index, Mongo plus a cache, a DB row plus an enqueued job) can partially
fail: the first write succeeds, the second throws, and the caller either
crashes after the first write already committed, or — worse — swallows the
second error and reports success. Either way the two systems are now out of
sync with nothing recording that it happened.

```js
await Order.create(orderDoc);           // committed
await redisClient.publish(CHANNEL, msg); // throws — nothing rolls back order
```

There is no free fix for the underlying problem — most polyrepo stacks have
no cross-store transaction — but never let the second write's failure pass
silently. Log it with enough identifying data to reconcile by hand or by a
backfill job, and prefer ordering the writes so the side effect that is
cheaper to redo (publish, enqueue) happens after the write that is expensive
to redo (the persisted record), so a retry after failure re-derives the side
effect from the record rather than duplicating the record.

## Idempotency

A consumer — a BullMQ job handler, a pub/sub subscriber, a webhook receiver
— can see the same message more than once: a worker crash after processing
but before acknowledging, an at-least-once delivery guarantee, a manual
retry. A handler that is not idempotent double-processes: sends a duplicate
email, double-decrements a counter, creates two records for one event.

Guard with a natural dedupe key that already exists in the payload (an order
id, a job id, an external event id) checked against what was already
processed, rather than assuming delivery-exactly-once from the transport:

```js
async function handle(job) {
  const already = await ProcessedEvent.exists({event_id: job.data.event_id});
  if (already) return; // safe replay
  await doWork(job.data);
  await ProcessedEvent.create({event_id: job.data.event_id});
}
```

## Contract versioning

When a payload or response shape must change in a way that cannot be
additive — a field's type changes, a field is removed and nothing can read
the old name — the two sides cannot deploy atomically across separate
repos. Carry an explicit version on the message or endpoint (`v: 2` in a
payload, `/v2/...` on an endpoint) so an old consumer can keep reading the
old shape from a still-running old producer during the rollout window,
rather than assuming every deploy lands instantaneously everywhere.

## Cross-refs

- Before shipping a change to a shared collection, channel, queue, or
  endpoint, run `znf:gate` to sweep every repo for producers/consumers this
  file did not enumerate.
- Before modifying an existing contract, run `znf:scout` to find its actual
  consumers and the tests that cover it — this skill teaches the shape of
  the risk, not which specific files depend on this one.
- The Mongo write side of a dual-write path — tenant scoping, `strict`
  schema traps — → the `mongo-data-safety` skill.
