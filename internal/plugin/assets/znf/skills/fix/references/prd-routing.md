<!-- Read when: Step 2b — the cause is confirmed and the bug may be live in production. -->

# Routing a production bug to `/hotfix`

A bug seen in production must be fixed on the base production runs from, with the response
decision (revert / disable / fix forward) and the post-deploy check that only `/hotfix` has.
`/fix` alone branches from the normal feature base, so a production bug fixed here ships late and
on the wrong base.

## Production evidence

Count it as production evidence only when the Step 0 source itself says production: a PostHog
event or Loki line from the prd environment, or a customer report on a prd tenant. A staging hit,
a local repro, or a failing CI run is not production evidence.

## The rule

Resolve the hotfix base after fetching, then look for the faulty line on it:

```bash
git -C <repo> fetch origin --tags
REF=$(zenify hotfix baseref <repo>)
git -C <repo> show "$REF":<file> | grep -n '<faulty line>'
```

| Case | Route |
|---|---|
| No production evidence | Stay in `/fix`, go on to Step 3. |
| The faulty line is on the hotfix base | `Skill(znf:hotfix)`. |
| The faulty line is not on the hotfix base (only on the normal base) | Stay in `/fix`, and say in the report: prd does not run this code yet. |
| The fault cannot be pinned to a line (data or config bug, several candidates) | `Skill(znf:hotfix)`; its step 4 asks the user which response to take. |
| `git show` fails because the path is missing on `$REF` (the file was renamed between the two bases) | Treat it as cannot be pinned → `Skill(znf:hotfix)`. Never conclude "prd does not run this code yet" from a missing path. |

## The handoff package

Pass all four to `/hotfix`, so its step 3 continues from the confirmed cause:

- the slug,
- the root cause and the paths of the evidence that confirmed it,
- the Step 0 red command, already run,
- the `track id:` that Step 0 opened — `/hotfix` keeps it instead of starting a second id.

Tell it: do not re-diagnose — its step 3 skips to step 4 when handed off from `/fix`.

## Repos whose hotfix base is the normal base

A repo whose `hotfix.baseStrategy` is `staging` or `standalone` resolves the hotfix base to the
same ref as the feature base (unless it declares `hotfixBaseRef`), so the line is always found there. Route it anyway: `/hotfix` still
adds the response decision (step 4) and the post-deploy check, which `/fix` does not have.
