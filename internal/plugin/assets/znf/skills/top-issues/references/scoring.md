# Scoring rubric — top-issues

A fixed rubric so two runs compare and a reader can recompute the ranking by hand from the numbers
the report prints. `now` = the value in the current window `<W>`, `prev` = the value in the window
right before it.

## Thresholds

A candidate is kept only above these minimums:

- Error candidate: ≥ 10 errors/window.
- Latency candidate: a route with ≥ 100 calls/window, and either p95 ≥ 2s or p95 ≥ 2 × the p95 of
  the previous window.

The same 10 errors/window threshold holds for money-path routes.

## Audience

Each issue belongs to exactly one group; take the highest that applies.

- **External**: people or systems outside the tenant hit the error directly — end customers
  messaging through a channel, partners or integrations calling an API or webhook, an SDK embedded
  in a customer's site. Recognised by a route name matching the generic keywords
  `external|webhook|public|callback|sdk|widget`, or by the root span of the sample trace coming from
  outside the system.
- **Tenant users**: a tenant's agents or admins hit the error in the UI or in an API the UI calls.
- **Internal**: only the system or the operations team sees it — service-to-service calls, workers,
  schedulers, jobs with retries. An internal route on a flow whose result External or Tenant users
  see takes the group of that flow.

## Data touch

- An issue touches data when a write method (POST/PUT/PATCH/DELETE) or a background job fails, and
  a log sample or trace shows a write that failed or repeated: write/insert/update/save failed,
  duplicate key, rollback, or a job dropped after its retries ran out.
- A route name alone, with no such log or trace, does not count as touching data.
- Always record it as *inferred*, unless a trace shows the specific record.

## Tenants affected

- The number of distinct tenants with error spans of the issue, counted with Tempo metrics grouped
  by the tenant attribute. The attribute name comes from the catalog's `attributes`: the key whose
  description contains "tenant"; none → `list_tempo_attribute_names`.
- Counted over the last **2h**: Tempo metrics refuse ranges of 3h or more; 2h keeps a margin and
  was measured working on prd. The report states that the number covers 2h.
- A series whose value is `nil` or empty is not a tenant. Every series `nil`, or no tenant
  attribute → record `unknown`; the level on this axis is then 0.
- **Level**: Local = 1 tenant; Several = 2–9 tenants; Widespread = ≥ 10 tenants, or ≥ 20% of the
  tenants with traffic in the same 2h.
- The denominator "tenants with traffic" needs one query per run.
- Print at most the 3 tenant ids with the most errors. An id is an identifier, not PII.

## Money path

A money path is a flow whose result creates, collects, deducts or refunds money: payment, billing,
invoices, orders, plans/subscriptions, wallet/top-up, and paid messaging (SMS, ZNS, ...).

- Recognised two ways: a route, span or app name matching the generic keywords
  `pay|billing|invoice|charge|order|subscription|wallet|topup|refund|sms|zns`; or the sample trace
  from the drill passing through such a service.
- A keyword match without a confirming trace is *inferred*.
- The money-path Impact floor applies only to write methods (POST/PUT/PATCH/DELETE) and background
  jobs. A GET route matching a keyword is still labelled money path but its Impact is not raised.
- Both keyword lists are generic words, never a workspace's own names.

## Impact

Take the highest level the issue meets.

- **5**:
  - touches data (records lost, written wrong or routed wrong); or
  - a money path at risk of charging wrong (double deduction, not collected, paid message sent
    repeatedly); or
  - tenants Widespread, audience External or Tenant users, and error rate ≥ 5%.
- **4**:
  - a write money path above the Thresholds, whatever the count; or
  - audience External above the Thresholds; or
  - tenants Several or Widespread; or
  - Tenant users with error rate ≥ 20% or ≥ 1000 errors/window; or
  - ≥ 2 services in the same call chain failing together.
- **3**:
  - Tenant users below level 4 (usually 1 tenant); or
  - Internal with ≥ 200 errors/window; or
  - queue backlog growing through the whole window; or
  - p95 ≥ 5s with audience External or Tenant users.
- **2**: Internal with < 200 errors/window; or p95 between 2s and 5s; or Internal with p95 ≥ 5s.
- **1**: only error log lines, with no request or job failing (the error was handled, then logged).

## Trend

- New: `prev = 0` and `now > 0`.
- A Loki candidate with `prev = 0` from the level-parsed query may be a log-format change (the
  previous window's lines were not JSON-parseable), not a new issue. Before scoring it New, count
  its top error message as a substring (`|= "<message prefix>"`) over the previous window; a
  non-zero count is its `prev`.
- Rising: `now ≥ 2 × prev`.
- Falling: `now ≤ 0.5 × prev`.
- Stable: everything else.

For an error candidate `now`/`prev` are error counts; for a latency candidate they are p95.

## Severity

| Impact | New / Rising | Stable | Falling |
|---|---|---|---|
| 5 | P0 | P1 | P1 |
| 4 | P1 | P1 | P2 |
| 3 | P1 | P2 | P3 |
| 2 | P2 | P3 | P3 |
| 1 | P3 | P3 | P3 |

## Ranking and tie-breaks

Sort all issues by Severity, then the tie-breaks; keep the first 5. Within the same Severity,
compare in order:

1. Higher Impact first.
2. A money-path issue before a non-money-path one.
3. Audience: External > Tenant users > Internal.
4. More tenants affected first (`unknown` counts as 0).
5. Trend: New > Rising > Stable > Falling.
6. Larger absolute error count in the window first.
7. `service + route` in alphabetical order.

Fewer than 5 issues → print only those; never pad.

## Risk if left (7 days)

Take the first that applies, in this order:

- High: Impact ≥ 3 and trend New or Rising; or the issue touches data; or the issue is on a write
  money path; or audience External and trend not Falling.
- Low: trend Falling.
- Medium: everything else.

## Remediation risk

- Low: a config or flag change, or rolling back one deploy; touches no data.
- Medium: a code fix in one repo, covered by tests.
- High: a shared-contract change, a data fix, several repos (must go through `/znf:gate`), or a
  code change on a money path.

## Confirmed or inferred

- Audience, money path and cause hypothesis: from a name or keyword alone → *inferred*; a trace or
  log sample that shows it → *confirmed*.
- Data touch: *inferred* unless a trace shows the specific record.

## Worked example

Hypothetical candidates, window 24h, all above the Thresholds.

| Candidate | Kind | now | prev | Calls/window | Tenants (2h) | Audience | Money path |
|---|---|---|---|---|---|---|---|
| A: `svc-a POST /orders` | errors | 366 | 18 | 9000 | 3 of 40 | Tenant users | yes, write |
| B: `svc-b GET /reports` | errors | 700 | 1500 | 23000 | 12 of 40 | Tenant users | no |
| C: `svc-c POST /webhook/inbound` | latency p95 (s) | 6.2 | 5.9 | 800 | `unknown` | External | no |

Trend:

- A: 366 ≥ 2 × 18 = 36 → Rising.
- B: 700 ≤ 0.5 × 1500 = 750 → Falling.
- C: 6.2 is neither ≥ 11.8 nor ≤ 2.95 → Stable.

Impact:

- A: write money path above the Thresholds → 4. No log or trace shows a failed or repeated write,
  so no data touch and not 5.
- B: Widespread (12 ≥ 10), but error rate 700 / 23000 ≈ 3% < 5% → not 5; Widespread → 4.
- C: External above the Thresholds → 4 (p95 ≥ 5s External would give only 3).

Severity: A = 4 Rising → P1; B = 4 Falling → P2; C = 4 Stable → P1.

Tie-break between A and C (both P1, both Impact 4): step 2, A is on a money path and C is not →
A first.

Ranking: 1. A (P1), 2. C (P1), 3. B (P2).

Risk if left: A High (write money path); C High (External, trend not Falling); B Low (Falling).
