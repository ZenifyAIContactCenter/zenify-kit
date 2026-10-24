# Queries — top-issues

The query behind each signal, measured on prd on 2026-10-24. Every name is a placeholder filled at
run time from `zenify catalog` or discovery: `<prom_uid>`, `<loki_uid>`, `<tempo_uid>` (datasource
uids), `<W>` (window), `<app_label>` and `<apps>` (Loki app label and its real values),
`<service>`, `<route>`, `<tenant_attr>`. Service ids given as arguments narrow each selector to
those services.

## a. Alerts

`alerting_rules_read` with `operation: list`, `states: [firing, pending]`, `limit_alerts: 5`.
A `null` result means no alert rules exist — record that, not "no issues".

## b. Errors by route

`query_prometheus` on `<prom_uid>`, `queryType: instant`, `endTime: now`:

```promql
topk(15, sum by (service, span_name) (increase(traces_spanmetrics_calls_total{span_kind="SPAN_KIND_SERVER", status_code="STATUS_CODE_ERROR"}[<W>])))
```

Run the identical query with `offset <W>` after the range selector for `prev`:

```promql
topk(15, sum by (service, span_name) (increase(traces_spanmetrics_calls_total{span_kind="SPAN_KIND_SERVER", status_code="STATUS_CODE_ERROR"}[<W>] offset <W>)))
```

The error rate used by the Impact rubric is the error ratio `zenify obs --route` prints in the drill.

## c. p95 by route

Same tool and arguments; values are in **seconds**:

```promql
topk(10, histogram_quantile(0.95, sum by (service, span_name, le) (rate(traces_spanmetrics_latency_bucket{span_kind="SPAN_KIND_SERVER"}[<W>]))) and on(service, span_name) sum by (service, span_name) (increase(traces_spanmetrics_calls_total{span_kind="SPAN_KIND_SERVER"}[<W>])) > 100)
```

Repeat with `offset <W>` on both range selectors for `prev`. A p95 of exactly `10` means at least
10s (the top bucket).

## d. Loki errors by app

- Find the level field first: `list_loki_label_names`; no level label → parse it with `| json`. A
  line with a text prefix before the JSON needs the JSON tail cut first (evidence-sources,
  "Measured dead ends").
- `query_loki_logs` on `<loki_uid>`, `queryType: instant`, `endRfc3339: now`; replace `level` with
  the discovered field name:

```logql
sum by (<app_label>) (count_over_time({<app_label>=~"<apps>"} | json | level=~"error|fatal" [<W>]))
```

- `prev`: the same query with `offset <W>` after `[<W>]`.
- Never filter on the substring "error": it counts info lines that mention the word.
- `query_loki_patterns` only on a candidate's app, with a window of at most 1h
  (`startRfc3339: now-1h`): one app over 1h already returns about 10 KB, mostly info patterns.

## e. Queue backlog

`list_prometheus_metric_names` on `<prom_uid>` with regex `(?i)(queue|backlog|waiting|lag)`.
A match → query it over `<W>` and check whether it grew through the whole window. No match → record
"not read: no queue metric".

## Tenant count (drill)

`query_tempo_metrics` on `<tempo_uid>`, `type: instant`, `start`/`end` = the last 2h (RFC3339):

```traceql
{ resource.service.name="<service>" && span.http.route="<route>" && kind=server && status=error } | count_over_time() by (span.<tenant_attr>)
```

Take `<route>` as its exact value from `list_tempo_attribute_values`. Denominator, once per run,
same 2h:

```traceql
{ kind=server } | count_over_time() by (span.<tenant_attr>)
```

Do not count `nil` or empty series as tenants.

## Deeplink

`generate_deeplink` with `resourceType: explore`, the datasource uid, the query as `queries` and
`timeRange` = the window. Put it under the issue's Evidence line.
