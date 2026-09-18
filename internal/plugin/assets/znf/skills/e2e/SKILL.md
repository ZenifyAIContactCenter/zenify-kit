---
name: e2e
description: Use when a plan task's deliverable is a user flow that changes an entity's state — creating, editing, or deleting a ticket, deal, contact, and the like — to decide whether it needs an E2E functional journey and to author one that passes `zenify e2e lint`. Covers Playwright journeys that drive the real UI and assert the domain outcome via an API re-fetch.
allowed-tools: Bash(zenify e2e *) Read Grep
---

# znf:e2e — deep E2E functional journeys (not shallow)

**Announce:** "Using znf:e2e to author a deep functional journey."

## Overview

A journey is a Playwright test that runs for real in Docker, drives a full user flow UI→BE, then
**asserts the domain outcome by re-fetching the entity through the API** — it never stops at "the
page loaded". Run it with `zenify e2e run`; the anti-shallow gate is `zenify e2e lint`.

## When to write a journey

Write one when a feature has a **real user flow through UI + BE that changes an entity's state**
(create/edit/delete a ticket, deal, contact…). Do NOT write one for copy changes, colour changes,
pure refactors, or FE-only changes that touch no data. This is a plan-time decision (`/cook`
Step 5), the same shape as the "which task deserves a browser run" decision for visual.

## Repo layout

- `.znf/e2e/e2e.config.json` — repo-owned: `{ "apiBaseUrl": "<staging api base>" }`.
- `.znf/e2e/<journey>.spec.ts` — the journey, importing fixtures: `import { test, expect } from '../../fixtures';`

Kit-provided generic fixtures (materialized at `run`): `page` (already logged in via storageState),
`apiClient` (authenticated HTTP — raw `Authorization` token), `cleanupTracker` (`.add(undo)`).

## Core principle — test-data lifecycle (each test owns its data)

On a **shared environment** (staging, many devs, many tests in parallel) each test creates and
cleans up its own data, isolated by a unique marker, never touching another test's data.

1. **Seed preconditions via the API, not the UI.** Only the **flow under test** goes through the
   browser; every precondition (contact, user, config…) is created/resolved with `apiClient`.
2. **Isolate with a unique per-run marker** (a `runId`/UUID in the subject/name) so parallel runs on
   the same staging never collide.
3. **Assert the domain outcome via an API re-fetch** (the "deep" part) — never stop at a UI signal.
4. **Clean up via the API at the end** (`cleanupTracker`), which runs **even when the test fails**
   (the harness guarantees this).

## Deep scenario template (required to pass lint)

Write the journey from the worked template in `references/deep-scenario-template.md` — it carries
every element the hard rules below require, with the trap each line avoids noted inline.

## Hard rules (lint fails on violation)

- A `// @domain-assert:<entity>` marker after the UI block; after it, an `apiClient` re-fetch plus an
  `expect` on a real field (not just `expect(page)`).
- **Exactly ONE** `// @domain-assert:<entity>` per scenario — lint FAILS on two or more. A
  multi-entity flow still re-fetches secondary entities with `apiClient` but adds no second marker:
  the marker names the scenario's PRIMARY domain outcome.
- No `networkidle`, no `waitForTimeout` — use web-first `await expect(...)`.
- Locators follow the Locator standard below — no xpath / `nth-child` / structural CSS.
- A `cleanupTracker.add(...)` (or `test.afterEach`) that removes every entity created.
- The scenario header carries an `FR-`/`SC-` reference.

## Doctrine rules (lint can't catch these yet — author + reviewer must hold them)

Mistakes AI/humans make often that the mechanical gate misses — violating them is shallow:

- **No `if`/`try` to hide failures.** `if (await x.isVisible())` around an action makes the test skip
  itself when the UI changes → a false green. Assert the expected state directly; let it go red.
- **Web-first assertions, always awaited.** `await expect(locator).toBeVisible()` — never
  `expect(await locator.isVisible()).toBe(true)`.
- **Assert user-visible behaviour + domain truth, never implementation** (CSS classes, internal
  state, DOM order).
- **Eventually-consistent backend?** If the entity isn't readable immediately after the write returns
  (search index, replica lag), use `await expect.poll(() => apiClient.get(...))` — never `waitForTimeout`.
- **Thin helpers / page objects**: locators + actions only, no assertions buried inside. At this
  scale, prefer fixtures over the Page Object Model.

## Locator standard (fixed order — testid is a fallback, NOT the default)

Prefer user-facing locators; `data-testid` is the last resort. Every journey follows this ladder, no
per-case debate:

1. `getByRole` (with name) · `getByLabel` · `getByPlaceholder` · `getByText` — **DEFAULT**.
2. `getByTestId` — **only** for an iframe/shadow-DOM element, a missing stable accessible name, or
   an i18n-generated label, with the reason noted inline (see references).
3. **NEVER**: xpath, `nth-child`, structural CSS — lint blocks these.

**Adding a testid to production code:** pass it via a prop on a shared component rather than
hardcoding it, and remove testids that are no longer used (see references).

## Run & verify

```bash
zenify e2e lint --repo <repo>            # mechanical gate, run first
zenify e2e run --repo <repo> --port <N>  # real run in Docker (dev-server on port N)
```

## References

- `references/deep-scenario-template.md` — the full worked journey to copy from, with the trap each line avoids.
- `references/rationale.md` — why deep beats shallow, why each doctrine rule and the locator ladder exist, what the harness already provides, and the upstream Playwright docs.
