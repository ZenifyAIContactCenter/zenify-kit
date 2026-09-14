---
summary: So ảnh chụp từng route với baseline, hoặc chụp lại baseline khi giao diện đổi có chủ ý.
---
## Khi nào dùng

Khi diff đổi giao diện và repo có `.znf/visual/routes.json`. Bước verify của `/znf:ship` chạy lệnh này trước khi dispatch agent `ui-verifier`. Golden diff bắt lệch ở vùng không liên quan tới thay đổi, còn `ui-verifier` đo phần tử vừa đổi.

## Kết quả

Lệnh đăng nhập một lần bằng các biến `E2E_DOMAIN`, `E2E_EMAIL`, `E2E_PASSWORD`, mở từng route trong `routes.json`, chờ font tải xong, che các vùng khai trong `mask`, rồi so ảnh toàn trang với baseline trong `.znf/visual/__snapshots__`. Lệch thì lệnh báo "visual mismatch" và trỏ tới ảnh diff trong `.znf/visual/__diff__`.

Mỗi route khai `name`, `path`, tùy chọn `waitFor` là selector cần chờ và `mask` là danh sách selector vùng động như badge thông báo hay đồng hồ.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--port` | Port dev server trên máy bạn. Bắt buộc. |
| `--repo` | Đường dẫn repo. Mặc định thư mục hiện tại. |
| `--update` | Chụp lại baseline thay cho so. Dùng khi giao diện đổi có chủ ý, rồi commit ảnh mới. |

## Ví dụ

```bash
zenify visual check --repo repos/contact-center-web --port 3312
zenify visual check --repo repos/contact-center-web --port 3312 --update
```

## Lưu ý

Cần Docker đang chạy. Thiếu Docker hoặc daemon chưa bật, lệnh báo rõ thay cho báo lệch ảnh.
