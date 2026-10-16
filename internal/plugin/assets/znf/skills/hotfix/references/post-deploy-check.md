# Hotfix — post-deploy check

Run after the user has merged and deployed the fix (step 10). The question is narrow: *did the
symptom stop in production, and did nothing nearby get worse*. Tool names, uids, app names and the
order of sources come from `znf:_shared/evidence-sources`; never type them from memory.

## Pick the checks by changed area

Run only the checks for the area the fix touched — a backend-only fix needs no PostHog query, a
web-only fix needs no queue metric. A fix that spans areas runs each matching check. Say which
areas you checked and why the others did not apply.

- **Backend** (route, service, query):
  - error rate of the changed route (or of the app, if the route has no metric of its own);
  - p95 latency of the changed route, from the spanmetrics histogram;
  - count of error lines in Loki for the app (`count_over_time` over the window).
- **Web** (screen, component, client code): PostHog exceptions for the affected screen or the
  error message the incident showed.
- **Queue** (producer, worker, job payload): the project's backlog or lag metric for the queue,
  found with `list_prometheus_metric_names` or read from the service catalog. No such metric →
  say so and confirm with `zenify redis-read` instead.

## Confirm production runs the fix

Before reading any number, check that the deployed build contains the change — an unchanged number
may only mean the deploy has not reached production. For a web bundle, fetch the served page, find
the chunk that holds the changed component and search it for a string the fix introduced (minified
names change; search a literal or property path). For a backend, use the project's version or
commit endpoint or its deploy log, if it has one. Neither available → say so.

## Compare two windows of equal length

Take the deploy time as the pivot. Compare a window **before** the deploy with a window of the
**same length after** it — for example 30 minutes each side, once the after-window is complete.
Unequal windows make a rate look better or worse than it is. Start the after-window at the deploy
time, never "the last hour", which mixes in pre-deploy traffic; the Tempo tools default to the past
hour when `start`/`end` are omitted, so always pass both. If the incident started long before
the deploy, take the before-window from inside the incident, not from a quiet period earlier.

A low-traffic route can show zero errors in both windows simply because nobody called it. Report
the request count next to the error count, so "0 errors" can be told apart from "0 requests".

## Errors that remain after the deploy

Group what remains by an attribute that identifies the client — user, client IP or user agent; read
the names with `list_tempo_attribute_names`, never assume them. Count distinct clients in each
window. Most clients stopped and a few repeat the old call → clients still running the old build (a
loaded web page keeps its bundle until it is reloaded); re-check later before acting. The same
spread of clients as before → the fix did not take effect.

## Record the result

Add one line per check to the hotfix report:

```
Post-deploy (<area>): <metric> before <value> → after <value> (windows <length>, pivot <deploy time>, clients <before> → <after> when the check has a client attribute)
```

The symptom metric did not drop, or a neighbouring one rose → tell the user at once; the response
from step 4 (revert, disable, fix forward) is open again. A check that could not run (source
missing, no metric for the queue) is reported as not run, with the reason — never as clean.
