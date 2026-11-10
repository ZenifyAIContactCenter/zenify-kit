---
summary: Gate fail-closed kiểm tra artifact UI-verify còn khớp fingerprint hiện tại.
---
## Khi nào dùng

`/znf:ship` bước 7 chạy lệnh này trước khi kết luận Shippable, sau khi trigger cơ học (`zenify ui-verify files` đếm ra khác 0) xác định diff có render gì đó.

## Kết quả

Bốn trạng thái:
- Render-set rỗng → "not required" (exit 0).
- Một dòng THÊM trong diff của nhánh (working tree so với merge-base với `--base`, gồm cả thay đổi chưa commit) nằm trong file render và mang marker `znf:ui-verify-ok: <lý do>` với lý do không rỗng → "waived: <lý do>" (exit 0). File chỉ đổi tên không tính là dòng thêm, nên marker cũ trong file bị đổi tên không miễn được.
- Artifact khớp fingerprint hiện tại có entry cho mọi viewport khai trong `uiVerify.viewports` (không khai thì một viewport bất kỳ mỗi màn hình), và mọi entry đều pass khi kit tính lại từ số đo layout và `failReason` đã ghi (bỏ qua `verdict` lưu trong file) → "verified" (exit 0).
- Còn lại → exit khác 0, `/znf:ship` đọc là "ui-verify required" và đánh dòng `look:` thành `❌ BLOCKED`, `Shippable: NO`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--repo` | Đường dẫn repo. Mặc định thư mục hiện tại. |
| `--base` | Base ref để so diff. Bắt buộc. |

## Ví dụ

```bash
zenify ui-verify check --repo repos/contact-center-web --base origin/staging
```
