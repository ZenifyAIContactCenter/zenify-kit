---
summary: Liệt kê hoặc bật/tắt các tính năng opt-in của kit trong overlay của workspace.
---
## Khi nào dùng

Khi bạn muốn bật một tính năng kit không chạy mặc định, ví dụ `github.issues` để `znf:ship` gắn issue GitHub (`Closes #N`) trước khi mở PR và `znf:nonstop` lên kế hoạch từ issue được giao.

## Kết quả

Không có đối số, lệnh in mỗi tính năng đã biết một dòng dạng `<key> on|off`. Với `<key> on|off`, lệnh ghi giá trị vào overlay rồi in lại đúng dòng `<key> on|off`. Giá trị khác `on`/`off`, key lạ, hoặc chỉ đưa một đối số đều báo lỗi và không ghi gì.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--workspace` | Thư mục workspace chứa `.zenify-overlay.yaml`. Mặc định là workspace đã resolve, không có thì dùng thư mục hiện tại. |

## Ví dụ

```bash
zenify feature
zenify feature github.issues on
zenify feature github.issues off
```

## Lưu ý

Mặc định mọi tính năng đều tắt. Trạng thái nằm ở khóa `features:` trong `.zenify-overlay.yaml` của workspace; lệnh giữ nguyên các khóa và comment khác trong file.
