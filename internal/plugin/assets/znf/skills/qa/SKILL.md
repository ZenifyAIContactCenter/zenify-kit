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
- Never `git push`. Commit only through `node /opt/sandbox/api-commit.mjs`, passing the `.znf/e2e/**` files you wrote.
- Always finish by writing `/out/qa-result.json`, success or not:
  `{"mode","key","repo","journey":bool,"green"?:bool,"reason"?:string,"acs":[{"n","kind"}]}`
  (`mode` = `scenario` | `journey` | `bind`; `acs` lists every `@ac` with its final kind).

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
3. `Bước` and `Thấy` use the exact words on screen. Find them by driving the sandbox app with `zenify browser`; never invent a label.
4. A `hỏi` line: ask the item owner with `zenify await needs-human --summary "<plain-language question>"`.
   The question carries no `@ac`, locator, API path, id or field name. At most 2 rounds; still
   unanswered → change the kind to `ngoài-e2e` with the reason on that line.
5. Open questions go under the open-questions section, never into a `Thấy` line.
6. Commit the scenario (and the config if created). Result: `"journey":false`.

## Mode `journey ZEN-<n>` (bug, tweak — before any fix exists)

Write `.znf/e2e/zen-<n>.spec.ts` following `znf:e2e` (deep scenario, domain assert, cleanup, locator ladder).

1. The header carries `// @scenario ZEN-<n>`. Every test title carries the `@ac:<n>` it proves; every `e2e` `@ac` gets a test.
2. Every string inside an `expect` is copied verbatim from that `@ac`'s `Thấy` lines.
3. Run `zenify e2e lint`, then `zenify e2e run --local --port $ZNF_SANDBOX_WEB_PORT` on base.
4. It must be red at the `expect` of an `@ac`. Red at a locator, navigation, seed or login step does
   not count: fix the journey (never the expect) and run again.
5. Two attempts without that red → delete the journey file, keep the scenario, write
   `{"journey":false,"reason":"<why>"}` (plus the common fields). Do not commit a journey that is green on base.
6. Red as required → commit; result `"journey":true`.

## Mode `bind ZEN-<n>` (feature — after the dev code exists)

The journey and scenario are frozen. You may add ONLY locators, navigation and seed data.

- Never edit the scenario file.
- Never change a literal inside an `expect`, and never delete, skip or weaken an `expect`.
- Run `zenify e2e lint`, then `zenify e2e run --local --port $ZNF_SANDBOX_WEB_PORT`.
- Green → commit; `"green":true`.
- Red → do not fix the `expect`; write `{"green":false,"failures":[…]}` with each failing `@ac` and
  its message. The failure is a finding for the dev run, not for you.

## References

- `references/scenario-template.md` — the scenario syntax the lint parser accepts, plus a full worked example (ZEN-123).
