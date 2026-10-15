---
summary: Đăng nhập trên host bằng harness e2e và ghi Playwright storage state để `znf:ui-verifier` nạp qua MCP.
---
## Khi nào dùng

Trước khi `znf:ui-verifier` kiểm tra UI trên một web app đang chạy local. Lệnh chạy trên host (không dùng Docker), nên origin là `http://localhost:<port>` và web app có thể trỏ vào hub local.

Lệnh cần Node (`npm`, `npx` có trên PATH). Lần chạy đầu tải `@playwright/test` 1.55.0 và chromium vào `~/.zenify/playwright/harness`, nên chậm và cần mạng; các lần sau dùng lại.

## Kết quả

Lệnh ghi storage state vào `~/.zenify/playwright/state.json` (quyền 0600), ghi qua file tạm rồi đổi tên, nên login lỗi thì state cũ vẫn còn nguyên. Khi manifest khai báo `broker:`, tài khoản đăng nhập lấy từ broker và bạn không điền gì. Manifest không có `broker:` thì thông tin đăng nhập đọc từ `E2E_DOMAIN`, `E2E_EMAIL`, `E2E_PASSWORD` trong `<workspace>/.zenify/.env` (env hiện có của tiến trình thắng nếu không rỗng); thiếu biến nào thì lệnh thoát với mã 2 và chỉ nêu tên biến. Thông tin đăng nhập không nằm trên argv và không được in ra.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--url` | Origin của web app; chỉ nhận `localhost`, `*.localhost` hoặc IP loopback (`127.0.0.1`, `::1`), origin khác bị từ chối. Phải có đúng một trong `--url` và `--port`. |
| `--port` | Port web app trên localhost (1..65535), tương đương `--url http://localhost:<port>`. |
| `--role` | Role tài khoản E2E xin từ broker: `administrator`, `supervisor`, `team_leader`, `agent`, `sale`, `sale_team_leader`. Ghi state vào `~/.zenify/playwright/state-<role>.json`. Cần manifest khai báo `broker:`, không có thì lệnh thoát mã 2 với "--role cần broker". |

Khi manifest khai báo `broker:`, lệnh không đọc `.zenify/.env`; không có `--role` thì dùng `administrator` và ghi `state.json` như cũ. Thông tin đăng nhập chỉ nằm trong env của tiến trình con, không ghi ra file `.env`.

## Ví dụ

```bash
zenify e2e login --port 3327
zenify e2e login --port 3327 --role agent
```
