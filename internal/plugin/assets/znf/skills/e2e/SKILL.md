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

## Nguyên tắc lõi — vòng đời test-data (mỗi test tự sở hữu data của nó)

Chuẩn thế giới cho E2E trên **môi trường chung** (staging nhiều dev + nhiều test song song): mỗi
test tự tạo–tự dọn data, cô lập bằng marker duy nhất, không đụng data của test khác.

1. **Seed điều kiện đầu vào QUA API, không qua UI.** Chỉ **luồng đang test** mới đi qua trình
   duyệt; mọi tiền đề (contact, user, config…) tạo/resolve bằng `apiClient` — nhanh, ổn định, không
   flaky. (Bấm 10 màn dựng data trong mỗi test là anti-pattern phổ biến nhất của test do AI viết.)
2. **Cô lập bằng marker duy nhất mỗi run** (`runId`/UUID nhét vào subject/tên). Nhờ vậy nhiều run
   song song trên cùng staging không giẫm nhau — tiền đề để bật `fullyParallel` khi suite lớn.
3. **Assert domain-outcome qua API re-fetch** (chỗ 'deep') — không dừng ở dấu hiệu UI.
4. **Dọn qua API ở cuối** (`cleanupTracker`), chạy **kể cả khi test fail** (harness lo, xem dưới).

`apiClient` không chỉ để assert — nó là **công cụ seed + cleanup**. Đây là ranh giới quyết định
'sâu vs hợt': UI kiểm *trải nghiệm người dùng*, API kiểm *sự thật domain*.

## Khuôn một scenario sâu (BẮT BUỘC qua lint)

```ts
import { test, expect } from '../../fixtures';

test('tạo ticket persist đúng field [FR-x, SC-y]', async ({ page, apiClient, cleanupTracker }) => {
  const runId = `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
  const marker = `[e2e-${runId}] smoke`;

  // --- seed điều kiện đầu vào QUA API (không qua UI) — xem "Vòng đời test-data" ---
  // Resolve requester TẠI RUNTIME; đừng pin id tĩnh trong config (id pin có thể bị
  // soft-delete trên staging chung → test rot âm thầm).
  const res = await apiClient.post('/v2/contacts/search', {
    data: { crm_type: 'contact', limit: 1, offset: 0 },
  });
  const requester = (await res.json()).data.list[0];

  // --- thao tác UI thật: CHỈ luồng đang test (tạo ticket) mới đi qua UI ---
  await page.goto(`/tickets/new-${runId}?customerId=${requester._id}`);
  // Subject: locator user-facing (placeholder), KHÔNG testid — xem "Chuẩn chọn locator".
  await page.getByPlaceholder('Nhập tiêu đề').first().fill(marker);
  // Editor TinyMCE render trong IFRAME → getByRole không xuyên được; testid chỉ làm
  // ANCHOR để frameLocator bấu vào (ca fallback hợp lệ #1, xem "Chuẩn chọn locator").
  await page.getByTestId('ticket-comment-editor')
    .frameLocator('iframe.tox-edit-area__iframe').locator('body').fill('E2E smoke content');
  const [resp] = await Promise.all([
    page.waitForResponse((r) => r.url().includes('/v2/ticket') && r.request().method() === 'POST'),
    page.getByTestId('ticket-submit-btn').click(),
  ]);
  // Tín hiệu thành công = POST trả 200. KHÔNG assert toast thành công: verified LIVE
  // toast tan quá nhanh, bắt không ổn định → journey flaky. Bằng chứng domain thật
  // là re-fetch bên dưới.
  expect(resp.status()).toBe(200);
  const id = (await resp.json())._id ?? (await resp.json()).data?._id;

  // --- assert domain-outcome qua API (đây là chỗ 'deep') ---
  // @domain-assert:ticket
  const got = await apiClient.get(`/v2/ticket/${id}`);
  const doc = (await got.json()).data ?? (await got.json());
  expect(doc.subject).toBe(marker);
  expect(typeof doc.status).toBe('number'); // status là Number (enum 1..5), KHÔNG phải string
  expect(doc.tenant_id).toBe(requester.tenant_id); // same-tenant guard, chống cross-tenant leak

  // --- cleanup: soft-delete theo run ---
  cleanupTracker.add(async () => {
    await apiClient.put(`/v2/ticket/${id}`, { data: { is_deleted: true } });
  });
});
```

## Quy tắc cứng (lint fail nếu vi phạm)

- Có `// @domain-assert:<entity>` sau khối thao tác UI; sau nó phải có `apiClient` re-fetch + `expect` trên field thật (không chỉ `expect(page)`).
- **Đúng MỘT** marker `// @domain-assert:<entity>` mỗi scenario — lint FAIL nếu có 2 marker trở lên. Flow đa-entity (assert cả ticket lẫn activity log liên quan) vẫn re-fetch bằng `apiClient` cho entity phụ, nhưng KHÔNG gắn thêm marker thứ hai — marker chỉ đặt tên domain-outcome CHÍNH của scenario.
- KHÔNG `networkidle`, KHÔNG `waitForTimeout` — dùng web-first `await expect(...)`.
- Selector theo **Chuẩn chọn locator** bên dưới (user-facing trước, testid là fallback có điều kiện) — KHÔNG xpath/`nth-child`/CSS bám cấu trúc.
- Có `cleanupTracker.add(...)` (hoặc `test.afterEach`) xoá entity đã tạo.
- Header scenario có ref `FR-`/`SC-`.

## Quy tắc doctrine (chuẩn thế giới — lint chưa bắt hết, người viết + review phải giữ)

Những lỗi test do AI/người viết hay mắc mà lint cơ học chưa chặn được — vi phạm là 'hợt':

- **KHÔNG `if`/`try` để giấu fail.** `if (await x.isVisible())` bọc thao tác = test tự bỏ qua khi
  UI đổi → xanh giả (lỗi #1 của test do AI viết). Assert thẳng trạng thái kỳ vọng; sai thì để đỏ.
- **Web-first assertion, luôn `await`.** `await expect(locator).toBeVisible()` — KHÔNG
  `expect(await locator.isVisible()).toBe(true)` (không auto-retry → flaky).
- **Assert thứ người dùng thấy + sự thật domain, KHÔNG assert implementation** (class CSS, state
  nội bộ, thứ tự DOM).
- **BE eventually-consistent?** Nếu entity chưa đọc được ngay sau khi write trả về (search index,
  replica trễ), dùng `await expect.poll(() => apiClient.get(...))` — KHÔNG `waitForTimeout`.
- **Helper/page-object mỏng**: chỉ locator + thao tác, KHÔNG nhét assert vào trong (giấu kỳ vọng
  khỏi test). Ở quy mô này ưu tiên **fixtures** hơn Page Object Model.

## Chuẩn chọn locator (thứ tự bắt buộc — testid là fallback, KHÔNG phải mặc định)

Chuẩn Playwright/Testing Library: ưu tiên locator theo góc nhìn người dùng, `data-testid` chỉ là
phương án cuối. Đây là chuẩn của kit — mọi journey theo đúng thang này, không cãi lại từng lần:

1. `getByRole` (kèm name) · `getByLabel` · `getByPlaceholder` · `getByText` — **MẶC ĐỊNH**. Bền với
   đổi DOM/CSS.
2. `getByTestId` — **CHỈ khi** trúng một trong ba ca fallback, và ghi rõ lý do ngay tại chỗ:
   - element trong **iframe / shadow-DOM** (getByRole không xuyên được) — vd TinyMCE editor;
   - **không có accessible name ổn định** để bấu vào;
   - **label do i18n sinh** và test phải độc-lập-ngôn-ngữ — `getByRole({name})` gãy khi đổi VI→EN,
     vd nút submit `t('common:action.create')`. Trong app i18n-100% như zenify, đây là ca hợp lệ.
3. **KHÔNG BAO GIỜ**: xpath, `nth-child`, CSS bám cấu trúc — lint chặn.

**Thêm testid vào code sản phẩm — hai luật giữ codebase sạch:**
- **Component dùng chung**: KHÔNG hardcode testid feature-specific lên nó. Truyền qua prop
  (vd `dataTestId`) để chỉ chỗ gọi cụ thể mới render — testid không rò sang 12+ nơi dùng khác.
- **Gỡ testid chết**: testid thêm rồi đổi selector không xài nữa phải gỡ khỏi codebase — đừng để
  lại làm người sau tưởng nó là anchor thật.

## Harness lo sẵn gì (đừng tự dựng lại)

- **Auth**: login một lần qua `auth.setup.ts` (setup project), lưu `storageState`; mọi test khởi
  động đã đăng nhập — KHÔNG login lại trong từng test. `apiClient` lấy token thô từ storageState.
- **Cleanup chạy kể cả khi fail**: `cleanupTracker` dọn ở teardown fixture (sau `use()`), nên run
  lỗi vẫn không rò data trên staging chung. Undo lỗi chỉ warn, không sập test khác.
- **Trace**: `trace: 'retain-on-failure'` — mỗi test fail có trace, mở bằng
  `npx playwright show-trace`. (Không `on-first-retry` vì suite chạy `retries=0`.)
- **Isolation**: mỗi test là browser context mới (cookies/storage sạch). `fullyParallel:false` mặc
  định an toàn cho staging chung; bật parallel khi mọi journey đã cô lập bằng marker (nguyên tắc #2).

## Chạy & verify

```bash
zenify e2e lint --repo <repo>            # cổng cơ học, chạy trước
zenify e2e run --repo <repo> --port <N>  # chạy thật trong Docker (dev-server ở port N)
```

## Chuẩn tham khảo (authoritative)

- Best Practices — playwright.dev/docs/best-practices (locator priority, web-first, isolation, trace)
- Locators & test-id — playwright.dev/docs/locators (`getByRole → … → getByTestId`; testid là cuối)
- API testing (seed / assert / cleanup qua API) — playwright.dev/docs/api-testing
- Auth `storageState` — playwright.dev/docs/auth · Fixtures — playwright.dev/docs/test-fixtures
