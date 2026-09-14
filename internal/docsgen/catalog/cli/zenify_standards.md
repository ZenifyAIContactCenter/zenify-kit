---
summary: Kiểm tra mỗi FR trong spec có một test thật trên đĩa mà plan đã khai.
---
## Khi nào dùng

Sau khi implement xong plan, trước khi ship. Skill `/znf:standards` gọi lệnh này rồi bổ sung nhận định xem test có thực sự kiểm đúng yêu cầu không.

## Kết quả

Lệnh đọc spec và plan, lấy các đường dẫn test plan khai, và kiểm từng FR có test tương ứng tồn tại trên đĩa. In số test path đã khai và danh sách finding theo mức HIGH, MEDIUM, INFO. Lệnh chỉ cảnh báo, không chặn. Cần cả spec lẫn plan, thiếu một trong hai lệnh bỏ qua.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--spec` | Đường dẫn file spec. |
| `--plan` | Đường dẫn file plan. |
| `--root` | Thư mục gốc để resolve đường dẫn test. Mặc định thư mục hiện tại. |
| `--json` | In JSON. |

## Ví dụ

```bash
zenify standards --spec docs/specs/zenify-kit/2026-09-14-kit-docs-site-design.md \
  --plan docs/plans/zenify-kit/2026-09-14-kit-docs-site.md --root repos/zenify-kit
```
