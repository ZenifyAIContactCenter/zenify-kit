---
summary: Kiểm tra cơ học các journey trong `.znf/e2e`, chặn journey thiếu re-fetch, assert hoặc cleanup.
---
## Khi nào dùng

Trước khi chạy `zenify e2e run` và trước khi mở PR có thêm hoặc sửa journey. Bước verify của `/znf:ship` chạy lệnh này khi repo có thư mục `.znf/e2e`.

## Kết quả

Lệnh đọc từng file `.spec.ts` và in vi phạm theo file và dòng. Các quy tắc gồm: đúng một marker `// @domain-assert:<entity>` mỗi scenario, sau marker phải có re-fetch qua API và một `expect` trên field thật, không dùng `networkidle` hoặc `waitForTimeout`, không dùng xpath hoặc `nth-child`, và phải có cleanup cho dữ liệu đã tạo.

Journey có dòng `// @scenario ZEN-<n>` còn chịu thêm bốn quy tắc gắn với kịch bản `.znf/e2e/scenarios/ZEN-<n>.md`: `scenario-link` (kịch bản phải tồn tại), `ac-coverage` (mỗi `@ac:<n>` loại `e2e` có một test gắn `@ac:<n>` trong tiêu đề), `expect-literal` (mọi chuỗi trong dòng `expect(...)` phải có nguyên văn trong kịch bản, trừ vai trò của `getByRole`, đối số của `getByTestId` và selector `locator(...)`), `no-skip` (cấm `test.skip`, `test.fixme`, `test.only`). Với journey này, `@ac:<n>` thay cho `FR-`/`SC-`. Kịch bản sai định dạng báo `scenario-parse`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--repo` | Đường dẫn repo cần kiểm. Mặc định thư mục hiện tại. |

## Ví dụ

```bash
zenify e2e lint --repo repos/contact-center-web
```
