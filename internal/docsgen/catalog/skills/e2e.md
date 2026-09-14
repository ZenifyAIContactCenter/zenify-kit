---
summary: Quyết định và viết journey E2E Playwright chạy thật từ UI tới BE, khẳng định kết quả domain bằng API re-fetch, qua được `zenify e2e lint`.
---
## Khi nào dùng

Khi một task có user flow đi qua UI và BE làm đổi trạng thái entity: tạo, sửa hoặc xóa ticket, deal, contact. Quyết định này đưa ra lúc lập plan trong `/znf:cook`. Không viết journey cho đổi chữ, đổi màu, refactor hoặc thay đổi chỉ ở FE không chạm dữ liệu.

## Cách hoạt động

1. Journey nằm ở `.znf/e2e/<journey>.spec.ts` trong repo, config ở `.znf/e2e/e2e.config.json` với `apiBaseUrl`. Kit cấp sẵn fixture `page` đã đăng nhập, `apiClient` đã xác thực và `cleanupTracker`.
2. Mỗi test tự tạo dữ liệu qua API với marker duy nhất theo lượt chạy; chỉ flow cần kiểm đi qua trình duyệt. Sau khối UI có đúng một marker `// @domain-assert:<entity>`, tiếp theo là một lần `apiClient` đọc lại entity và `expect` trên field thật. Cuối test đăng ký dọn dẹp qua API; dọn dẹp chạy cả khi test fail.
3. `zenify e2e lint` fail khi thiếu marker hoặc re-fetch, có hai marker trong một scenario, dùng `networkidle` hoặc `waitForTimeout`, dùng xpath, `nth-child` hoặc CSS cấu trúc, thiếu cleanup, hoặc header scenario không nhắc `FR-` hay `SC-`.
4. Locator theo thứ tự cố định: `getByRole`, `getByLabel`, `getByPlaceholder`, `getByText` là mặc định; `getByTestId` chỉ khi phần tử nằm trong iframe hoặc shadow DOM, không có accessible name ổn định, hoặc label sinh từ i18n.

## Ví dụ

```bash
zenify e2e lint --repo <repo>
zenify e2e run --repo <repo> --port <N>
```

## Lưu ý

Harness đăng nhập một lần qua `storageState`, giữ trace khi test fail, và mặc định chạy tuần tự trên staging chung. Không dùng `if` hoặc `try` để che fail; assertion luôn là web-first và được `await`.
