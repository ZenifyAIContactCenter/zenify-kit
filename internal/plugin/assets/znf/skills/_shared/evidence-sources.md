# Evidence sources — shared reference

**Last updated:** 2026-10-18

The one place a skill learns how to pull runtime evidence: logs, traces and metrics through the
Grafana MCP server, product analytics through the PostHog MCP server, and stored state through
`zenify db-read` / `zenify redis-read`. All of it is read-only; writes and scripts go through
`zenify data run` (`data-scripts.md`). The broker MCP servers exist when
the manifest declares a `broker:` (registered by `zenify up`); read their server names from the
session's MCP tool list, where each tool name starts `mcp__<server>__`. Everything project-specific — datasource uids, app names, attribute and metric
names — is discovered at run time or read from the project's service catalog (`zenify catalog`); never hard-code it.

## Before the first query

1. Run `zenify catalog <id> --prd|--stg` (`<id>` = the service or datastore; omit it for the whole
   catalog). It prints YAML with the datasource uids and the real app, `service.name` and metric
   names. Use those names as given. `no catalog configured` (exit 5), exit 2 (unknown id, which
   lists the valid ones) or any other error → steps 2-3 below. A `catalog is <N>s old` line on
   stderr means the broker served a stale copy; the names are still usable.
2. `list_datasources` → the uid of each Loki, Tempo and Prometheus datasource. Tool arguments
   take the **uid**, not the display name.
3. `list_loki_label_names`, then `list_loki_label_values` on the app label it returns → the real
   app names. The app name in Loki, the `service.name` in Tempo and the repo name can all differ;
   map them once here instead of guessing.

A live name the catalog lacks: run `zenify catalog drift [--prd|--stg]` (table; exit 6 means drift,
0 clean). Report the drift to the user and carry on with the live name; do not edit the catalog.

## Order by symptom

- **Errors** — Loki errors for the app over a narrow window → take the trace id on a failing log
  line → `get_tempo_trace`. For the web front end, PostHog exceptions first.
- **Slow** — p95 by route from the Tempo spanmetrics histogram (`query_prometheus` on
  `traces_spanmetrics_latency_bucket`) → TraceQL `{ duration > 2s }` on that route
  (`search_tempo_traces`) → `get_tempo_trace` (see Output size).
- **Queue / background job** — find the project's backlog or lag metric with
  `list_prometheus_metric_names`, query it, then confirm the real keys with `zenify redis-read`.
- **User-side** (what a user saw or clicked) — PostHog: events, sessions, exceptions.

Then confirm stored state with `zenify db-read [--prd]` and `zenify redis-read [--prd]`.

## Pivots between sources

- **Log → trace**: the trace id printed on the log line → `get_tempo_trace`.
- **Job → producer**: a consumer span carries a span link to the producer span; the attribute that
  holds it is project-specific — find it with `list_tempo_attribute_names`, then open the linked
  trace.
- **Trace → log**: search the trace id as a plain string across every app
  (`{<app-label>=~".+"} |= "<trace id>"`), not only the app you expect — bounded to a few minutes
  around the trace's own start and end, with a line limit.
- **Analytics → trace**: take tenant, user and time window from the PostHog event, then search
  Tempo with the matching resource or span attributes over that window.

Attribute and label names come from `list_tempo_attribute_names` and the Loki label/field
discovery above, or from the service catalog — never from memory.

## Measured dead ends

- `get_tempo_trace` takes `trace_id`, not `traceId`.
- The Tempo tools reject unknown arguments, and neither `search_tempo_traces` nor
  `query_tempo_metrics` has a `limit`: narrow with the query and `start`/`end` (RFC3339). Omitting
  `start`/`end` searches the past hour. `query_tempo_metrics` takes `type`: `instant` or `range`.
- Datasource arguments take the uid from `list_datasources`; a name fails.
- The Loki app name differs from the repo name and from Tempo's `service.name`; read it with
  `list_loki_label_values`.
- Spanmetrics carry no HTTP status. Count statuses with Tempo metrics:
  `{ } | count_over_time() by (span.http.status_code)`.
- A Tempo metrics query covers at most 3 h; split longer ranges.
- An instant `query_prometheus` needs `endTime`.
- Tempo keeps traces only for its retention window (measured on one deployment: about two days).
  An empty Tempo search or metrics query for an older window means "aged out", not "no errors":
  for older windows use spanmetrics in Prometheus and Loki. Find the edge by searching one hour a
  day or two back.
- TraceQL span kind is the intrinsic `kind=server`; `span.kind=server` matches nothing and returns
  an empty series.
- Match a route by its exact value read from `list_tempo_attribute_values` (e.g.
  `span.http.route`), not a regex guess.
- Deferred MCP tools are called by their full name `mcp__<server>__<tool>`; the bare tool name
  fails with "No such tool". If the server is still connecting, wait and search again rather than
  switching approach.
- `traces_spanmetrics_latency_bucket` buckets are in **seconds** (`le` 0.001 … 10, then `+Inf`),
  so a p95 of exactly `10` means "at least 10 s, capped by the top bucket", not 10 ms. The
  service label is `service`; server spans are `span_kind="SPAN_KIND_SERVER"` (measured
  2026-10-15).
- `| json` fails on a log line with a text prefix before the JSON. Cut the JSON tail first with
  ``| regexp `(?P<payload>\{.*\})$` ``, then `| line_format "{{.payload}}" | json`.

## PostHog exec

The PostHog MCP server exposes **one** tool, `exec`, which takes one command per call:

1. `search <regex>` — find the tool you need.
2. `info <tool>` — read its input schema.
3. `call [--json] <tool> <json>` — run it.

There is no separate MCP tool per PostHog tool, so never write one as `mcp__…__<name>`. The broker
key is read-only, so no write tool is available.

## Data handling

Query counts, ids and shapes. Never copy a value — message content, phone number, email, token,
customer name — into a spec, PR, memory, ledger or report; replace it with `<REDACTED>`. Scrub
inside the command (a `sed` filter) so the raw value never enters the conversation.

## Output size

- Narrow every query: one app, the smallest time window, a line or row limit.
- An MCP result cannot be shell-redirected. Narrow the call by its own arguments first (time
  range, limit, filters); a large result (a full `get_tempo_trace` is about 200 KB) is saved to a
  file by the harness on its own — `jq` that file for the fields you need, never read it whole.
- `zenify redis-read` prints a truncation notice on stderr when it cut the reply; narrow and
  re-run rather than reasoning over a partial list.
- `zenify db-read` `--limit` defaults to 50 and caps at 200.

## When a source is missing

If an MCP server is absent or fails to connect, say which server is missing — it is a connection
problem, not proof the data does not exist. Use `zenify db-read` / `zenify redis-read` if they
answer the question; otherwise stop and report the gap. Do not guess the missing evidence and do
not read `.env` files for credentials.

## Role dev on production

If the session-start line says `role=dev`, production allows only:

db-read: collections, count (no --filter), explain, indexes, stats, tables, sql: EXPLAIN SELECT without subquery, DESCRIBE <table>, SHOW INDEX|COLUMNS|TABLES|CREATE TABLE|TABLE STATUS; redis-read: EXISTS LLEN SCAN SCARD TTL TYPE ZCARD

- `explain` for dev on prd returns the `queryPlanner` plan only — no `executionStats`.
- `count` takes no `--filter`.
- `db-read --target audit` (Mongo, prd only) is lead-only.
- Grafana logs, traces and metrics stay available. For row data, read staging, or ask a lead to run the prd step.
