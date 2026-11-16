---
summary: Gate fail-closed kiểm tra artifact UI-verify còn khớp fingerprint hiện tại.
---
## Khi nào dùng

`/znf:ship` bước 7 chạy lệnh này trước khi kết luận Shippable, sau khi trigger cơ học (`zenify ui-verify files` đếm ra khác 0) xác định diff có render gì đó.

## Kết quả

Năm trạng thái:
- Render-set rỗng → "not required" (exit 0).
- Diff mang marker `// znf:ui-verify-ok: <lý do>` → "waived: <lý do>" (exit 0).
- Có artifact hợp lệ khớp fingerprint hiện tại → "verified" (exit 0). Artifact chỉ hợp lệ khi mọi screen có verdict `pass`; một screen `fail` làm cả artifact không hợp lệ.
- Có bản ghi `zenify ui-verify blocked` cho fingerprint hiện tại (không có artifact hợp lệ) → `infra-blocked: <mã>`. Khi `ZNF_AUTONOMOUS=1` thoát 0 và không tạo await, để PR vẫn mở với dòng cảnh báo; ngoài autonomous thì như dưới (exit khác 0).
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
