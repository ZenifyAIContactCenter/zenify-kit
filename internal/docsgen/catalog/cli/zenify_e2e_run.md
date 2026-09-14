---
summary: Chạy journey E2E trong Docker, trỏ vào dev server đang mở trên máy bạn.
---
## Khi nào dùng

Khi journey đã qua `zenify e2e lint` và dev server của worktree đang chạy. Lệnh tự lint lại trước khi chạy, journey hời sẽ dừng ngay.

## Kết quả

Lệnh khởi động container Playwright đã ghim phiên bản, đăng nhập một lần bằng các biến `E2E_DOMAIN`, `E2E_EMAIL`, `E2E_PASSWORD` trong môi trường, rồi chạy mọi journey trong `.znf/e2e`. Kết quả Playwright in ra màn hình.

Repo cần file `.znf/e2e/e2e.config.json`. Thiếu file này, lệnh báo repo chưa cấu hình e2e và dừng.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--port` | Port dev server trên máy bạn. Bắt buộc. |
| `--repo` | Đường dẫn repo. Mặc định thư mục hiện tại. |

## Ví dụ

```bash
zenify e2e run --repo repos/contact-center-web --port $(git -C repos/contact-center-web/.worktrees/my-task config --get wt.port)
```

## Lưu ý

Cần Docker đang chạy. Nếu thiếu Docker, chạy `zenify doctor` để xem môi trường.
