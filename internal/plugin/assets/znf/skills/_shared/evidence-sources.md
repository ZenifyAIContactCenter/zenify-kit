# Evidence sources — shared reference

**Last updated:** 2026-10-18

The one place a skill learns how to pull runtime evidence: logs, traces and metrics via `zenify obs`
or the Grafana MCP server, product analytics through the PostHog MCP server, and stored state through
`zenify db-read` / `zenify redis-read`. All of it is read-only; writes and scripts go through
`zenify data run` (`data-scripts.md`). Broker MCP servers exist when the manifest declares a
`broker:` (`zenify up` registers them); read their server names from the
session's MCP tool list (`mcp__<server>__<tool>`). Everything project-specific — datasource uids,
app names, attribute and metric names — is discovered at run time or read from the project's service catalog (`zenify catalog`); never hard-code it.

## Before the first query

1. Run `zenify catalog <id> --prd|--stg` (`<id>` = the service or datastore; omit it for the whole
   catalog). It prints YAML with the datasource uids and the real app, `service.name` and metric
   names. Use those names as given. Exit 2 (unknown id) lists the valid ids: pick the match, or
   run it without an id and match on `repo`. `no catalog configured` (exit 5) or any other error →
   steps 2-3 below. A `catalog is <N>s old` line on stderr means a stale copy; the names still work.
2. `list_datasources` → the uid of each Loki, Tempo and Prometheus datasource. Tool arguments
   take the **uid**, not the display name.
3. `list_loki_label_names`, then `list_loki_label_values` on the app label it returns → the real
   app names. The app name in Loki, the `service.name` in Tempo and the repo name can all differ;
   map them once here instead of guessing.

A live name the catalog lacks: run `zenify catalog drift [--prd|--stg]` (table; exit 6 = drift or
an unread source, 0 clean). Report it and carry on with the live name; do not edit the catalog. A row with an `ERROR` value is a source the broker could not read: report it as
that, not as catalog drift.

## Order by symptom

- **Errors / Slow** — start with `zenify obs <id> --route "<METHOD path>" --prd|--stg`: rate,
  p50/p95/p99, error ratio, status mix, sample slow and failing traces, PII-masked, capped at 32 KB.
  Then `zenify obs --trace <trace-id> --prd|--stg` on a sampled trace (1-32 hex chars). For more
  detail use the Grafana MCP tools: Loki errors over a narrow window → trace id → `get_tempo_trace`
  (see Output size), or `query_prometheus` on `traces_spanmetrics_latency_bucket` → TraceQL
  `{ duration > 2s }` (`search_tempo_traces`). Exit 5 (no `obs`) → the MCP path alone.
  Web front end: PostHog exceptions first.
- **Queue / background job** — find the project's backlog or lag metric with
  `list_prometheus_metric_names`, query it, then confirm the real keys with `zenify redis-read`.
- **User-side** (what a user saw or clicked) — PostHog: events, sessions, exceptions.

Then confirm stored state with `zenify db-read [--prd]` and `zenify redis-read [--prd]`.

## Pivots between sources

- **Log → trace**: the trace id printed on the log line → `get_tempo_trace`.
- **Job → producer**: a consumer span carries a span link to the producer span; the attribute that
  holds it is project-specific — find it with `list_tempo_attribute_names`, then open the linked
  trace.
- **Trace → log**: read each service's Loki app from `zenify catalog`, then search every one of them
  for the 32-char id and the short original id, bounded to a few minutes around the trace and with
  a line limit. `zenify obs --trace` already does this.
- **Analytics → trace**: take tenant, user and time window from the PostHog event, then search
  Tempo with the matching resource or span attributes over that window.

Attribute and label names come from `list_tempo_attribute_names`, Loki label discovery or the
service catalog — never from memory.

## Measured dead ends

- `get_tempo_trace` takes `trace_id`, not `traceId`.
- The Tempo tools reject unknown arguments, and neither `search_tempo_traces` nor
  `query_tempo_metrics` has a `limit`: narrow with the query and `start`/`end` (RFC3339). Omitting
  `start`/`end` searches the past hour. `query_tempo_metrics` takes `type`: `instant` or `range`.
- Spanmetrics carry no HTTP status. Count statuses with Tempo metrics:
  `{ } | count_over_time() by (span.http.status_code)`.
- Tempo metrics queries of 3 h or more are refused: `zenify obs` splits the range for you; a direct
  MCP call must split it by hand. Tempo can drop a trace id's leading zeros: pad it to 32 chars to
  compare, and search logs for both the 32-char id and the original short id.
- Loki refuses a selector matching every value of a label (regex `.+`): name real apps.
- An instant `query_prometheus` needs `endTime`.
- Tempo keeps traces only for its retention window (one deployment: about two days). An empty Tempo search or metrics query for an older window means "aged out", not "no errors":
  for older windows use spanmetrics in Prometheus and Loki. Find the edge by searching an hour a day
  or two back.
- TraceQL span kind is the intrinsic `kind=server`; `span.kind=server` matches nothing and returns
  an empty series.
- Match a route by its exact value from `list_tempo_attribute_values` (e.g.
  `span.http.route`), not a regex guess.
- Deferred MCP tools are called by their full name `mcp__<server>__<tool>`; a bare name fails with
  "No such tool". If the server is still connecting, wait and search again.
- `traces_spanmetrics_latency_bucket` buckets are in **seconds** (`le` 0.001 … 10, then `+Inf`),
  so a p95 of exactly `10` means "at least 10 s, capped by the top bucket", not 10 ms. The
  service label is `service`; server spans are `span_kind="SPAN_KIND_SERVER"` (measured
  2026-10-15).
- `| json` fails on a log line with a text prefix before the JSON. Cut the JSON tail first with
  ``| regexp `(?P<payload>\{.*\})$` ``, then `| line_format "{{.payload}}" | json`.
- `alerting_rules_read` (`operation: list`) returns `null` when the instance has no
  Grafana-managed alert rules — that is "no rules", not a connection failure; rank from metrics
  and logs instead.
- Counting Loki errors with a substring filter (`|~ "(?i)error"`) matches info lines that merely
  mention the word, inflating counts by orders of magnitude; filter on the parsed level field (a
  level label or `| json | level=~"error|fatal"`).
- A level-parsed Loki count of 0 is "no errors" unless that message's own lines failed to parse;
  check the message, not the app's whole volume: `|= "<message>" | json | __error__!="" | drop
  __error__, __error_details__` (the `drop` keeps the metric query accepted). Most of them failing
  (measured: 518 of 518 after a log format change) means the count is not trustworthy; only then
  compare the message by substring in both windows.

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
- `zenify db-read` `--limit`: default 50, cap 200.

## When a source is missing

If an MCP server is absent or fails to connect, say which server is missing — it is a connection
problem, not proof the data is absent. Use `zenify db-read` / `zenify redis-read` if they
answer the question; otherwise stop and report the gap. Do not guess the missing evidence or read
`.env` files for credentials.

## Role dev on production

If the session-start line says `role=dev`, production allows only:

db-read: collections, count (no --filter), explain, indexes, stats, tables, sql: EXPLAIN SELECT without subquery, DESCRIBE <table>, SHOW INDEX|COLUMNS|TABLES|CREATE TABLE|TABLE STATUS; redis-read: EXISTS LLEN SCAN SCARD TTL TYPE ZCARD

- `explain` for dev on prd returns the `queryPlanner` plan only — no `executionStats`.
- `count` takes no `--filter`.
- `db-read --target audit` (Mongo, prd only) is lead-only.
- `db-read --target lumi` (lumi-agent MySQL, sql|tables) is lead-only on prd; staging is open. Read lumi MySQL with it, not ssh and not `data run`.
- Grafana logs, traces and metrics stay available. For row data, read staging, or ask a lead to run the prd step.
