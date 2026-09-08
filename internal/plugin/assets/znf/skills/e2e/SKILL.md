---
name: e2e
description: Author a deep E2E functional journey — a Playwright test that runs for real in Docker, drives a real user flow UI→BE, and asserts the domain outcome by re-fetching the entity through the API. Use when a plan task's deliverable is a user flow that changes an entity's state (create/edit/delete ticket, deal, contact…), to decide whether the task needs a journey and to write one that passes `zenify e2e lint`.
allowed-tools: Bash(zenify e2e *) Read Grep
---

# znf:e2e — viết E2E functional journey sâu (không 'hợt')

**Announce:** "Using znf:e2e to author a deep functional journey."

Capability E2E functional của kit: một journey Playwright chạy thật trong Docker giả lập người
dùng đi trọn luồng UI→BE, rồi **assert domain-outcome bằng cách re-fetch entity qua API** — không
dừng ở "trang load". Chạy bằng `zenify e2e run`; chặn 'hợt' bằng `zenify e2e lint`.

## Khi nào viết journey (trigger)

Viết khi tính năng có **luồng người dùng thật xuyên UI + BE làm đổi trạng thái một entity**
(tạo/sửa/xoá ticket, deal, contact…). KHÔNG viết cho: đổi copy, đổi màu, refactor thuần, thay đổi
chỉ-FE không chạm dữ liệu. Quyết định này thuộc plan-time (/cook Step 5), cùng dạng quyết định
"task nào đáng browser-run" của visual.

## Bố cục repo

- `.znf/e2e/e2e.config.json` — repo sở hữu: `{ "apiBaseUrl": "<api base staging>", "contactId": "<id contact có sẵn trên tenant E2E>" }`.
- `.znf/e2e/<journey>.spec.ts` — journey code, import fixtures: `import { test, expect } from '../../fixtures';`

Fixtures generic do kit cấp (materialize lúc `run`): `page` (đã login qua storageState),
`apiClient` (HTTP đã auth — `Authorization` token thô), `cleanupTracker` (`.add(undo)`).

## Khuôn một scenario sâu (BẮT BUỘC qua lint)

```ts
import { test, expect } from '../../fixtures';

test('tạo ticket persist đúng field [FR-x, SC-y]', async ({ page, apiClient, cleanupTracker, cfg }) => {
  const runId = `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
  const marker = `[e2e-${runId}] smoke`;

  // --- thao tác UI thật ---
  await page.goto(`/tickets/new-${runId}?customerId=${cfg.contactId}`);
  await page.getByTestId('ticket-subject-input').fill(marker);
  // ...điền field bắt buộc khác...
  const [resp] = await Promise.all([
    page.waitForResponse((r) => r.url().includes('/v2/ticket') && r.request().method() === 'POST'),
    page.getByTestId('ticket-submit-btn').click(),
  ]);
  await expect(page.getByText('Tạo ticket mới thành công')).toBeVisible();
  const id = (await resp.json())._id ?? (await resp.json()).data?._id;

  // --- assert domain-outcome qua API (đây là chỗ 'deep') ---
  // @domain-assert:ticket
  const got = await apiClient.get(`/v2/ticket/${id}`);
  const doc = (await got.json()).data ?? (await got.json());
  expect(doc.subject).toBe(marker);
  expect(typeof doc.status).toBe('number'); // status là Number (enum 1..5), KHÔNG phải string

  // --- cleanup: soft-delete theo run ---
  cleanupTracker.add(async () => {
    await apiClient.put(`/v2/ticket/${id}`, { data: { is_deleted: true } });
  });
});
```

## Quy tắc cứng (lint fail nếu vi phạm)

- Có `// @domain-assert:<entity>` sau khối thao tác UI; sau nó phải có `apiClient` re-fetch + `expect` trên field thật (không chỉ `expect(page)`).
- KHÔNG `networkidle`, KHÔNG `waitForTimeout` — dùng web-first `await expect(...)`.
- Selector ổn định: `getByTestId`/`getByRole`/`getByLabel`/`getByPlaceholder` — KHÔNG xpath/nth-child.
- Có `cleanupTracker.add(...)` (hoặc `test.afterEach`) xoá entity đã tạo.
- Header scenario có ref `FR-`/`SC-`.

## Chạy & verify

```bash
zenify e2e lint --repo <repo>            # cổng cơ học, chạy trước
zenify e2e run --repo <repo> --port <N>  # chạy thật trong Docker (dev-server ở port N)
```
