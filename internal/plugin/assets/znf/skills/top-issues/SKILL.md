---
name: top-issues
description: Use when asked for the system's current top issues, a health or incident overview across services, or what is broken or degrading right now and how to handle it — before any one bug is known. For one known bug use znf:fix.
argument-hint: "[--stg] [--since <duration>] [service-id...]"
allowed-tools: Read Write Bash(zenify catalog*) Bash(zenify obs*)
---

**Report, never fix.** This skill reads Grafana, ranks the top 5 current issues with a fixed
rubric, proposes remediation and hands the issue the user picks to `znf:fix`. It changes no code,
config or data, and hands nothing off until the user chooses.

## Step 0: Args

- Env: `--prd` by default; `--stg` switches to staging.
- Window `<W>`: 24h by default, compared with the 24h right before it. `--since <duration>` changes
  it, at most 48h (the cap of `zenify obs --since`).
- Any service ids given narrow every query to those services.

## Step 1: Sources

- Follow `_shared/evidence-sources.md` "Before the first query": `zenify catalog --prd|--stg` for
  datasource uids, Loki app names and Tempo service names; on a catalog error, its fallback.
- Tenant attribute `<tenant_attr>`: the key under the catalog's `attributes` whose description
  contains "tenant"; none → `list_tempo_attribute_names`.
- Read the MCP server name from the session's tool list (`mcp__<server>__<tool>`); never hard-code it.
- A server is missing → "When a source is missing" in that file.

## Step 2: Collect

Read the five signals with the formulas in `references/queries.md`; b–d each read the current
window and the previous one (`offset <W>`):

- a. firing or pending alerts;
- b. errors by service + route;
- c. p95 by route, above the call-volume threshold;
- d. Loki error lines by app, filtered on the parsed level field;
- e. queue backlog, when a queue metric exists.

Record each signal as "read" or "not read: <reason>". Empty, `null` or an error is never
"no issues". `alerting_rules_read` returning `null` means no alert rules — record it and continue.

- Keep a candidate only above the thresholds in `references/scoring.md` (Thresholds).
- Narrow every query per "Output size": `topk`, line limits, smallest window.
- Call only read MCP tools (query, list, get, search). Never create, update or delete anything in
  Grafana.
- Cap: 40 MCP calls per run. Budget: sources ≤ 4 (datasource fallback, tenant attribute); signals
  a–e ≤ 12; drill ≤ 4 per issue (tenant, deeplink, log or pattern, fallback), ≤ 20 for 5 issues;
  tenant denominator 1. `zenify obs` is a CLI command and does not count. At the cap, stop
  collecting and list what was not read at the top of the report.

## Step 3: Merge

Candidates on the same call chain (e.g. the same route on two services that call each other) or
with the same error pattern are one issue.

## Step 4: Score

Apply `references/scoring.md`: audience, tenant level, money path, data touch, Impact, trend,
Severity, then the tie-breaks; keep the top 5. Fewer than 5 issues → print only those, never pad.
Print every `now`/`prev` number used to score, so a reader can recompute the ranking by hand.

## Step 5: Drill

For each top-5 issue:

1. Route issue: `zenify obs <id> --route "<METHOD path>" --prd|--stg --since <W>`, then
   one sampled trace via `zenify obs --trace`, read as error-level messages only
   (`references/queries.md`, "Trace error messages"). Never read `.logs.lines` whole or print a
   request body: `obs` does not mask them. No route → a narrowed log sample or pattern
   (`references/queries.md`).
2. Issue with spans: count tenants with errors over the last 2h (`references/queries.md`, Tenant).
   Read the tenant denominator once per run.
3. A Grafana deeplink for the evidence.

PII: follow "Data handling" — never copy message content, phone, email, customer name or token
into chat or file; write `<REDACTED>`. Tenant ids are identifiers and may be printed (top 3).

## Step 6: Remediation

Each issue carries these `**Label:** value` lines:

- Signal (now vs prev)
- Audience: External / Tenant users / Internal, with the reason and *confirmed* or *inferred*
- Tenants: count over 2h, level (Local / Several / Widespread), % of tenants with traffic, up to 3
  tenant ids; or "unknown" with the reason
- Money path: yes or no, with the reason and *confirmed* or *inferred*
- Impact
- Risk if left (7 days)
- Cause hypothesis, *confirmed* or *inferred*
- Remediation: immediate mitigation + root fix
- Remediation risk, with how to roll it back
- Repo to fix in
- Handoff command
- Evidence (query + Grafana deeplink)

## Step 7: Report

- Header: env, window, run time, and each signal read or not read with the reason.
- Summary table of the top 5: rank, severity, service + route, audience, tenants, money path, trend.
- One detail section per issue (Step 6).
- Prose in the project's language; identifiers, queries and labels stay as they are.

Print it in chat, then Edit or create a file (`_shared/harness-tools.md`)
`docs/exports/_cross/YYYY-MM-DD-top-issues-<env>.md`; a rerun on the same day overwrites that day's
file. Write is for this file only.

## Step 8: Handoff

Ask the user to choose between options: issue #1–#5, or "none".

- A pick → invoke `Skill(znf:fix)` with the service, route, time window, trace id and the
  evidence lines of that issue.
- A P0 affecting prd → also suggest `/znf:hotfix` for the user to type; that skill is user-only.
- "none" or no answer → stop.

## References

- `references/scoring.md` — read at Step 2 (thresholds) and Step 4: the full rubric, risk scales,
  confirmed/inferred rule and a worked example.
- `references/queries.md` — read at Step 2 and Step 5: the measured query for each signal, the
  tenant count and the deeplink.
