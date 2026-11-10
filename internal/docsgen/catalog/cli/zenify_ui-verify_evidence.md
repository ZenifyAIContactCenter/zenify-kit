---
summary: Gom bằng chứng UI-verify của fingerprint hiện tại (screen, style, a11y, ảnh lệch) vào một file Markdown local.
---
## Khi nào dùng

Chạy sau `zenify ui-verify check` để reviewer mở một file là thấy đủ bằng chứng. File `.znf/ui-verify/<fp>-evidence.md` có state của `check`, bảng screen × viewport (verdict, overflow, đường dẫn tuyệt đối tới ảnh), kết quả lint style, tổng kết a11y từ `visual check` (chỉ tham khảo) và thư mục ảnh lệch nếu có.

Lệnh in đường dẫn file ra stdout và thoát 0 dù không có artifact hay kết quả fail; chỉ config `uiVerify` sai (exit 2) hoặc không ghi được file (exit 1) mới khác 0. Không gọi mạng, không tải gì lên, không nhúng ảnh. File a11y hoặc `routes.json` hỏng được ghi `unreadable` thay vì làm lệnh dừng.

## Ví dụ

```bash
zenify ui-verify evidence --repo . --base "$(zenify wt base)"
```
