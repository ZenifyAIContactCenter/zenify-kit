---
summary: Thêm một route vào danh mục visual-regression của repo mà không sửa JSON bằng tay.
---
## Khi nào dùng

Khi `ui-verifier` đề xuất một route mới cho danh mục và phiên chính ở `/znf:ship` quyết định thêm nó. Lệnh ghi route vào `.znf/visual/routes.json` đúng định dạng, giữ nguyên các route cũ và thứ tự của chúng.

## Kết quả

Lệnh validate danh sách sau khi thêm. `name` hoặc `path` đã tồn tại thì lệnh thoát mã 2 và file không đổi. Chưa có `routes.json` thì lệnh tạo file với đúng một route. Sau khi thêm, lệnh in ba dòng: số route hiện có, lệnh `zenify visual check ... --update` để chụp baseline, và lệnh `git add -f` cho `routes.json` cùng các file baseline của route.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--name` | Tên route, khớp `[A-Za-z0-9_-]+`, không trùng route khác. Bắt buộc. |
| `--path` | Đường dẫn route, bắt đầu bằng `/`, không trùng route khác. Bắt buộc. |
| `--repo` | Đường dẫn repo. Mặc định thư mục hiện tại. |
| `--wait-for` | Selector cần chờ trước khi chụp. |
| `--mask` | Selector vùng động cần che. Lặp cờ để che nhiều vùng. |
| `--aria` | Selector gốc để so snapshot cấu trúc aria. Không khai thì route chỉ so ảnh. |

## Ví dụ

```bash
zenify visual add-route --repo repos/contact-center-web --name ticket-list --path /tickets --mask '[data-testid="app-header"]'
```

## Lưu ý

Lệnh chỉ sửa `routes.json`. Baseline chỉ có sau khi bạn chạy `zenify visual check --update`, và phải `git add -f` cả `routes.json` lẫn baseline thì `visual check` mới chấp nhận.
