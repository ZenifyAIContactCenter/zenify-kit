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

**Deep vs shallow is the whole point:** the UI checks the *user experience*, the API re-fetch checks
the *domain truth*. A journey that only asserts UI state is shallow, and a live run will out it.

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
   browser; every precondition (contact, user, config…) is created/resolved with `apiClient` — fast,
   stable, not flaky. (Clicking through ten screens to build data per test is the single most common
   AI-authored-test anti-pattern.)
2. **Isolate with a unique per-run marker** (a `runId`/UUID in the subject/name) so parallel runs on
   the same staging never collide — the precondition for turning on `fullyParallel` as the suite grows.
3. **Assert the domain outcome via an API re-fetch** (the "deep" part) — never stop at a UI signal.
4. **Clean up via the API at the end** (`cleanupTracker`), which runs **even when the test fails**
   (the harness guarantees this).

## Deep scenario template (required to pass lint)

```ts
import { test, expect } from '../../fixtures';

test('create ticket persists subject/status/tenant [FR-x, SC-y]', async ({ page, apiClient, cleanupTracker }) => {
  const runId = `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
  const marker = `[e2e-${runId}] smoke`;

  // Seed the precondition via API (not UI). Resolve the requester at runtime — a static id
  // pinned in config can be soft-deleted and rot the test silently.
  const res = await apiClient.post('/v2/contacts/search', {
    data: { crm_type: 'contact', limit: 1, offset: 0 },
  });
  const requester = (await res.json()).data.list[0];

  // Real UI actions: only the flow under test (creating a ticket) goes through the UI.
  await page.goto(`/tickets/new-${runId}?customerId=${requester._id}`);
  await page.getByPlaceholder('Nhập tiêu đề').first().fill(marker); // user-facing locator, no testid
  // The TinyMCE editor renders inside an IFRAME, so getByRole cannot reach it; the testid is only
  // an anchor for frameLocator (valid fallback case #1 — see the Locator standard).
  await page.getByTestId('ticket-comment-editor')
    .frameLocator('iframe.tox-edit-area__iframe').locator('body').fill('E2E smoke content');
  const [resp] = await Promise.all([
    page.waitForResponse((r) => r.url().includes('/v2/ticket') && r.request().method() === 'POST'),
    page.getByTestId('ticket-submit-btn').click(),
  ]);
  // Success signal = POST returns 200. Do NOT assert the success toast: it tears down too fast to
  // catch reliably and makes the journey flaky. The API re-fetch below is the real domain evidence.
  expect(resp.status()).toBe(200);
  const id = (await resp.json())._id ?? (await resp.json()).data?._id;

  // Assert the domain outcome via API (this is the "deep" part).
  // @domain-assert:ticket
  const got = await apiClient.get(`/v2/ticket/${id}`);
  const doc = (await got.json()).data ?? (await got.json());
  expect(doc.subject).toBe(marker);
  expect(typeof doc.status).toBe('number'); // status is a Number (enum 1..5), not a string
  expect(doc.tenant_id).toBe(requester.tenant_id); // same-tenant guard against cross-tenant leaks

  // Clean up via API (soft-delete), scoped to this run.
  cleanupTracker.add(async () => {
    await apiClient.put(`/v2/ticket/${id}`, { data: { is_deleted: true } });
  });
});
```

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
  itself when the UI changes → a false green (the #1 AI-authored-test mistake). Assert the expected
  state directly; let it go red when wrong.
- **Web-first assertions, always awaited.** `await expect(locator).toBeVisible()` — never
  `expect(await locator.isVisible()).toBe(true)` (no auto-retry → flaky).
- **Assert user-visible behaviour + domain truth, never implementation** (CSS classes, internal
  state, DOM order).
- **Eventually-consistent backend?** If the entity isn't readable immediately after the write returns
  (search index, replica lag), use `await expect.poll(() => apiClient.get(...))` — never `waitForTimeout`.
- **Thin helpers / page objects**: locators + actions only, no assertions buried inside. At this
  scale, prefer fixtures over the Page Object Model.

## Locator standard (fixed order — testid is a fallback, NOT the default)

Prefer user-facing locators; `data-testid` is the last resort. Every journey follows this ladder, no
per-case debate:

1. `getByRole` (with name) · `getByLabel` · `getByPlaceholder` · `getByText` — **DEFAULT**. Resilient
   to DOM/CSS changes.
2. `getByTestId` — **only** for one of three fallback cases, with the reason noted inline:
   - the element is inside an **iframe / shadow DOM** (getByRole can't reach it) — e.g. the TinyMCE editor;
   - there is **no stable accessible name** to anchor on;
   - the **label is i18n-generated** and the test must be language-independent — `getByRole({name})`
     breaks when VI→EN, e.g. a submit button from `t('common:action.create')`. In an i18n-100% app
     like zenify this is a valid case.
3. **NEVER**: xpath, `nth-child`, structural CSS — lint blocks these.

**Adding a testid to production code — two rules to keep the codebase clean:**
- **Shared component**: never hardcode a feature-specific testid on it. Pass it via a prop (e.g.
  `dataTestId`) so only the specific call site renders it — the testid doesn't leak onto the 12+ other usages.
- **Remove dead testids**: a testid added then abandoned (selector changed) must be removed — don't
  leave it to look like a real anchor.

## What the harness provides (don't rebuild it)

- **Auth**: logs in once via `auth.setup.ts` (setup project), saves `storageState`; every test starts
  authenticated — no per-test login. `apiClient` reads the raw token from storageState.
- **Cleanup runs even on failure**: `cleanupTracker` drains in fixture teardown (after `use()`), so a
  failed run leaks no data on shared staging. A failing undo only warns; it doesn't break other tests.
- **Trace**: `trace: 'retain-on-failure'` — every failed test has a trace; open it with
  `npx playwright show-trace`. (Not `on-first-retry`, because the suite runs `retries=0`.)
- **Isolation**: each test gets a fresh browser context. `fullyParallel:false` is the safe default for
  shared staging; turn on parallel once every journey is marker-isolated (principle #2).

## Run & verify

```bash
zenify e2e lint --repo <repo>            # mechanical gate, run first
zenify e2e run --repo <repo> --port <N>  # real run in Docker (dev-server on port N)
```

## References (authoritative)

- Best Practices — playwright.dev/docs/best-practices (locator priority, web-first, isolation, trace)
- Locators & test-id — playwright.dev/docs/locators (`getByRole → … → getByTestId`; testid last)
- API testing (seed / assert / cleanup via API) — playwright.dev/docs/api-testing
- Auth `storageState` — playwright.dev/docs/auth · Fixtures — playwright.dev/docs/test-fixtures
