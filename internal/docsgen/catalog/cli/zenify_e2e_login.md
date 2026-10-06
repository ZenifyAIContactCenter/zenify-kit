---
summary: Đăng nhập trên host bằng harness e2e và ghi Playwright storage state để `znf:ui-verifier` nạp qua MCP.
---
## Khi nào dùng

Trước khi `znf:ui-verifier` kiểm tra UI trên một web app đang chạy local. Lệnh chạy trên host (không dùng Docker), nên origin là `http://localhost:<port>` và web app có thể trỏ vào hub local.

Lệnh cần Node (`npm`, `npx` có trên PATH). Lần chạy đầu tải `@playwright/test` 1.55.0 và chromium vào `~/.zenify/playwright/harness`, nên chậm và cần mạng; các lần sau dùng lại.

## Kết quả

Lệnh ghi storage state vào `~/.zenify/playwright/state.json` (quyền 0600), ghi qua file tạm rồi đổi tên, nên login lỗi thì state cũ vẫn còn nguyên. Thông tin đăng nhập chỉ đọc từ biến môi trường `E2E_DOMAIN`, `E2E_EMAIL`, `E2E_PASSWORD`; chúng không nằm trên argv và không được in ra. Thiếu biến nào thì lệnh thoát với mã 2 và chỉ nêu tên biến.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--url` | Origin của web app; chỉ nhận `localhost`, `*.localhost` hoặc IP loopback (`127.0.0.1`, `::1`), origin khác bị từ chối. Phải có đúng một trong `--url` và `--port`. |
| `--port` | Port web app trên localhost (1..65535), tương đương `--url http://localhost:<port>`. |

## Ví dụ

```bash
zenify e2e login --port 3327
```
