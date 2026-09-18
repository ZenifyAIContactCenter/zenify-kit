# E2E — the deep scenario template

The reference journey a `.znf/e2e/<journey>.spec.ts` is written from. Every element it shows is
required by one of the hard rules in `SKILL.md`; the inline comments say which trap each line
avoids.

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
