---
summary: Chạy journey E2E trong Docker, trỏ vào dev server đang mở trên máy bạn.
---
## Khi nào dùng

Khi journey đã qua `zenify e2e lint` và dev server của worktree đang chạy. Lệnh tự lint lại trước khi chạy, journey hời sẽ dừng ngay.

## Kết quả

Lệnh khởi động container Playwright đã ghim phiên bản, đăng nhập một lần bằng tài khoản E2E lấy từ broker khi manifest khai báo `broker:`, hoặc bằng các key `E2E_DOMAIN`, `E2E_EMAIL`, `E2E_PASSWORD` trong `.zenify/.env` khi không có broker, rồi chạy mọi journey trong `.znf/e2e`. Kết quả Playwright in ra màn hình.

Repo cần file `.znf/e2e/e2e.config.json`. Thiếu file này, lệnh báo repo chưa cấu hình e2e và dừng.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--port` | Port dev server trên máy bạn. Cần đúng một trong `--port` hoặc `--base-url`. |
| `--base-url` | Origin cần test thay cho dev server, ví dụ `https://staging.example.com`. Chỉ nhận origin (bỏ path, host không phân biệt hoa thường) nằm trong `baseUrls` của `.znf/e2e/e2e.config.json`; ngoài danh sách hoặc thiếu `baseUrls` thì lệnh thoát mã 2 trước khi chạy Docker. |
| `--local` | Chạy trên Playwright của host (cùng harness với `zenify e2e login`) thay vì Docker, không cần Docker. Chỉ nhận app loopback (`--port`, hoặc `--base-url` là localhost/127.0.0.1). Lệnh vẫn lint trước, rồi chép `.znf/e2e` vào harness và chạy `npx playwright test`. Bỏ qua `npm install` khi harness đã có `@playwright/test` 1.55.0, bỏ qua tải chromium khi `PLAYWRIGHT_BROWSERS_PATH` đã có `chromium-<revision>` đúng revision mà harness cần (đọc từ `playwright-core/browsers.json`), nên chạy được khi không có mạng. |
| `--prepare` | Chỉ đi cùng `--local`: chuẩn bị harness (npm và chromium) rồi thoát mã 0, không chạy test. Image worker gọi lúc build. |
| `--repo` | Đường dẫn repo. Mặc định thư mục hiện tại. |
| `--role` | Role tài khoản E2E xin từ broker (`administrator`, `supervisor`, `team_leader`, `agent`, `sale`, `sale_team_leader`). Cần manifest khai báo `broker:`; không có broker thì lệnh thoát mã 2. Không có `--role` mà có broker thì dùng `administrator`. |

## Ví dụ

```bash
zenify e2e run --repo repos/contact-center-web --port $(git -C repos/contact-center-web/.worktrees/my-task config --get wt.port)
```

## Lưu ý

Không có `--local` thì cần Docker đang chạy. Nếu thiếu Docker, chạy `zenify doctor` để xem môi trường.
