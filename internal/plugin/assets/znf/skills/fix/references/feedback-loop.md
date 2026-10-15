<!-- Adapted from mattpocock/skills diagnosing-bugs (MIT). Read when: building the Step 0 loop, or Step 2/5/6 need it. -->

# The feedback loop

Step 0 ends with **one named command, already run**, whose output is red on exactly the symptom the
user described. Every later step re-runs it: Step 2 confirms the cause with it, Step 5 turns it into
a regression test, Step 6 watches it go green.

## Redact first

Scrub inside the command, so raw output never enters the conversation: pipe it through a filter,
e.g. `| sed -E 's/(token|password|secret|key)=[^& ]+/\1=<REDACTED>/Ig'`. Replace with `<REDACTED>`
every secret — token, password, API key, cookie, connection string — every piece of customer PII —
phone, email, name, address — and every tenant or account id. The same applies to anything written
to a file, the ledger, a PR body or a fixture; fixtures and replayed payloads use synthetic data.
The loop reads credentials from environment variables (`$API_TOKEN`), never from a pasted value.

`KEY=value` lines never carry a secret or PII value, `[DEBUG-` log lines never print a secret or PII
field, and a HAR file is scrubbed before you read it.

## The ladder

Try the rungs top-down; stop at the first one that goes red on the right symptom.

1. **Failing test** at the seam that touches the bug.
2. **`curl`** against the worktree's own port (the one `wt` allocated) or another local target.
3. **CLI with a fixture** — the real entry point fed a saved input.
4. **Replay a captured payload** — the request, job or message that failed, redacted, against the
   worktree's own port or a local target only.
5. **Throwaway harness** — a short script that calls the failing function directly.
6. **Property / fuzz loop** — generate inputs until one fails.
7. **`git bisect run <cmd>`** — when it worked before and you have a command that tells good from bad.
8. **Old vs new** — same input against the good build and the bad build, diff the outputs.
9. **Numbered manual steps** — last resort: steps for the user to perform by hand, each returning
   one `KEY=value` line you can read back. No script. This is the one exception to "already run" and
   "you can run it yourself": the user's pasted `KEY=value` lines are the red evidence, and Step 6
   asks the user to repeat the same steps and paste the lines again.

Read-only means log reads, running an existing failing test, and GET or HEAD requests — nothing
else. Those need no worktree. A request with a side effect, or a payload replay, runs only against
the worktree's own port or a local target. Anything that writes to a shared or production
environment needs the user's approval and goes through a dry-run script the user runs; the agent
never runs it. A rung that writes a file or starts a server needs the worktree first.

## Tighten the loop

- **Fast** — seconds, not minutes. Narrow to the one test, the one request, the smallest fixture.
- **Sharp** — one assertion on the exact symptom, not "some exception was thrown".
- **Stable** — same result every run. A loop that is red one time in three proves nothing.

## Flaky bugs

Raise the reproduction rate before you guess: repeat N times in a loop, force the timing or the
concurrency (sleep, parallel runs, a smaller pool), pin the seed and the clock. Guess only once the
loop is red reliably.

## Done when

- [ ] It goes red.
- [ ] It is red on the symptom the user described, not a neighbouring error.
- [ ] It is stable across repeated runs.
- [ ] It is fast enough to re-run after every change.
- [ ] You can run it yourself, without the user — except rung 9, where the user's pasted
      `KEY=value` lines are the evidence.

## Evidence sources via the broker

For a production or staging symptom, pull evidence in this order: Loki, Tempo, `zenify db-read` /
`zenify redis-read`, PostHog. All of it is read-only. The MCP servers `zenify-grafana` and
`zenify-posthog` exist when the manifest declares a `broker:` (registered by `zenify up`); without
one, fall back to the local credentials the project documents. Redact as above before anything
enters the conversation.

1. **Errors by service over a time range (Loki).** Call `list_loki_label_names` first and use the
   service label it returns; do not assume a label name. Then `query_loki_logs` with
   `{<service-label>="<svc>"} |= "error"` over the last 30 minutes.
2. **One slow or failing request (Tempo).** Find candidates with `search_tempo_traces` using
   TraceQL, e.g. `{resource.service.name="<svc>" && name="<route>" && duration > 3s}`, then fetch
   one with `get_tempo_trace` by trace id. A full trace is large (about 200 KB): save it to a file
   and aggregate with `jq`, never print it.
3. **Latency by route (Prometheus).** `query_prometheus` on `traces_spanmetrics_latency_bucket`
   (measured 2026-10-15). Buckets are in **seconds** (`le` 0.001 … 10, then `+Inf`), so a p95 of
   exactly `10` means "at least 10 s, capped by the top bucket", not 10 ms. The service label is
   `service`; server spans are `span_kind="SPAN_KIND_SERVER"`.

Then check the stored state with `zenify db-read [--prd]` and `zenify redis-read [--prd]`, and
user-side behaviour in PostHog (`zenify-posthog`).

## Regression seam

At Step 5 you choose the seam yourself — the test that runs the bug the way the real caller does —
or decide there is none and write `no seam: <why>`. This is the agent's call; do not ask the user.

## Debug logs

Every temporary log line carries a unique prefix `[DEBUG-xxxx]` (four hex characters, one per
session), so removal is a search, not a memory. Before reporting, both must return nothing:

```bash
rg '\[DEBUG-' <changed files>
git diff <base> | rg '\[DEBUG-'
```

## Performance bugs

Measure a **baseline** as a number before changing anything; the loop is a command that prints that
number. Then bisect — by commit, by input size, or by stage — until the number moves.

## No loop

If the whole ladder yields no red command: stop. List what you tried and what each rung printed,
then ask for one of (a) access to an environment that reproduces it, (b) a redacted artifact from the
failure (log, payload, HAR), (c) permission to add temporary `[DEBUG-xxxx]` logging where it fails.
Do not move on to hypotheses without a loop.
