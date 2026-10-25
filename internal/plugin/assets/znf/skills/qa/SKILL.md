---
name: qa
description: Use when running as the QA worker for a work item — writes the frozen acceptance scenario, a journey that is red on base, or binds that journey to finished code without bending its expects.
argument-hint: "scenario ZEN-<n> | journey ZEN-<n> | bind ZEN-<n>"
allowed-tools: Bash(zenify *) Bash(git *) Bash(node *) Read Grep Write Edit
---

# znf:qa — frozen acceptance scenarios and journeys

**Announce:** "Using znf:qa in <mode> mode for ZEN-<n>."

The scenario is written BEFORE any dev code exists and is frozen from then on. Its value is that the
dev run cannot bend the test to fit the code. Every rule below protects that.

## Isolation (all modes)

- Never read the dev run's branch, plan, diff or commits. Read only the code at base and the running sandbox app.
- Never `git push`. Commit only with (repo, branch and base sha exactly as the run prompt gives them;
  the item branch is `zen-<n>`, `--repo` is the primary repo below):
  `node /opt/sandbox/api-commit.mjs --repo <repo> --branch zen-<n> --message "test(e2e): <mode> ZEN-<n>" --base-sha <base sha> <path>...`
  Each `<path>` is a file you wrote under `.znf/e2e/`. `--base-sha` creates the branch when it does not exist yet.
- Exit 2 = a path was rejected: fix the path list, never move files outside `.znf/e2e/`. Any other
  non-zero exit: stop and put it in `reason`. Success prints the commit sha: record it as `"commit"`.
- Always finish by writing `/out/qa-result.json`, success or not:
  `{"mode","key","repo","journey":bool,"green"?:bool,"reason"?:string,"commit"?:string,"acs":[{"n","kind"}],"failures"?:[{"ac":n,"message":string}]}`
  (`mode` = `scenario` | `journey` | `bind`; `acs` lists every `@ac` with its final kind; `failures`
  only for a red bind: one entry per failing `@ac`, `message` = the Playwright assertion message).

## Role and login (all modes)

- `Vai:` names the role in plain words. Map it to one broker E2E role: `administrator`, `supervisor`,
  `team_leader`, `agent`, `sale` or `sale_team_leader` (for example `nhân viên` → `agent`). Call it `<role>` below.
- Driving the app with `zenify browser`: first `zenify e2e login --port $ZNF_SANDBOX_WEB_PORT --role <role>`,
  then `zenify browser -s=<session> open` and `zenify browser -s=<session> state-load <path the login printed>`
  before you navigate (`znf:run`, autonomous sandbox notes).
- Every journey run is `zenify e2e run --local --port $ZNF_SANDBOX_WEB_PORT --role <role>`, so it runs as the scenario's role.

## Primary repo

- Bug: the repo where you see the failure when you reproduce it.
- Tweak or feature: the repo that owns the changed screen; a web screen means `contact-center-web`.
- Scenario and journey live only in that repo's `.znf/e2e/`. Write `repo` into `/out/qa-result.json`.
- No `.znf/e2e/e2e.config.json` there yet: create it in the same commit, with `baseUrls` =
  `["http://127.0.0.1:$ZNF_SANDBOX_WEB_PORT"]` (the port value, resolved) and `apiBaseUrl` = `http://127.0.0.1:3001` (be).

## Mode `scenario ZEN-<n>`

Write `.znf/e2e/scenarios/ZEN-<n>.md` from `references/scenario-template.md`.

1. One `@ac` per acceptance line of the item; copy the acceptance sentence verbatim into the heading.
2. Kind is exactly one of `e2e`, `hỏi`, `ngoài-e2e`. A `ngoài-e2e` line says how it is checked instead (ui-verifier or unit).
3. `Bước` and `Thấy` use the exact words on screen. Find them by driving the sandbox app with `zenify browser`,
   logged in as `<role>` (see Role and login); never invent a label.
4. A `hỏi` line: ask the item owner with `zenify await needs-human --summary "<plain-language question>"`.
   The question carries no `@ac`, locator, API path, id or field name. At most 2 rounds; still
   unanswered → change the kind to `ngoài-e2e` with the reason on that line. The reply arrives as
   `resume <id> answered: <text>`.
5. Open questions go under the open-questions section, never into a `Thấy` line.
6. Commit the scenario (and the config if created). Result: `"journey":false`.

## Mode `journey ZEN-<n>` (bug, tweak — before any fix exists)

A bug or tweak gets only this run, so it writes the scenario too: run Mode `scenario` steps 1-5 first,
then write `.znf/e2e/zen-<n>.spec.ts` following `znf:e2e` (deep scenario, domain assert, cleanup, locator ladder).

1. The header carries `// @scenario ZEN-<n>`. Every test title carries the `@ac:<n>` it proves; every `e2e` `@ac` gets a test.
2. Every string inside an `expect` is copied verbatim from that `@ac`'s `Thấy` lines.
3. Run `zenify e2e lint`, then `zenify e2e run --local --port $ZNF_SANDBOX_WEB_PORT --role <role>` on base.
   An attempt is one `zenify e2e run --local` on base after `zenify e2e lint` passes; lint failures and their fixes are not attempts.
4. It must be red at the `expect` of an `@ac`. Red at a locator, navigation, seed or login step does
   not count: fix the journey (never the expect) and run again.
5. Two attempts without that red → delete the journey file, commit the scenario (and the config if
   created), write `{"journey":false,"reason":"<why>"}` (plus the common fields). Do not commit a journey that is green on base.
6. Red as required → commit the scenario, the journey (and the config if created); result `"journey":true`.

## Mode `bind ZEN-<n>` (feature — after the dev code exists)

The scenario is frozen; the journey's expects come from it, verbatim.

1. No `.znf/e2e/zen-<n>.spec.ts` yet (a feature: the `qa` run wrote only the scenario) → write it
   following `znf:e2e`: header `// @scenario ZEN-<n>`, one test per `e2e` `@ac` with `@ac:<n>` in its
   title, and every string inside an `expect` copied verbatim from that `@ac`'s `Thấy` lines.
2. From then on, and whenever the journey already exists, you may add or change ONLY locators,
   navigation and seed data.

- Never edit `.znf/e2e/scenarios/ZEN-<n>.md` and never pass it to `api-commit.mjs`. zenie compares it
  with its content at the `qa` commit and fails the run on any difference.
- Never change a literal inside an `expect`, and never delete, skip or weaken an `expect`.
- Run `zenify e2e lint`, then `zenify e2e run --local --port $ZNF_SANDBOX_WEB_PORT --role <role>`.
- Red at a locator, navigation, seed or login step is yours: fix the journey (never the `expect`) and run again.
- Green → commit the journey; `"green":true`.
- Red at the `expect` of an `@ac` → still commit the journey (the dev run's autofix runs it), do not fix the `expect`; write `{"green":false,"failures":[{"ac":<n>,"message":"<assertion message>"}]}`
  (plus the common fields), one entry per failing `@ac`. The failure is a finding for the dev run, not for you.

## References

- `references/scenario-template.md` — the scenario syntax the lint parser accepts, plus a full worked example (ZEN-123).
