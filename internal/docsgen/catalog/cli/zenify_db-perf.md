---
summary: Quét tĩnh một diff để tìm anti-pattern query DB, chia finding thành BLOCKING và ADVISORY.
---
## Khi nào dùng

Khi diff thêm hoặc đổi một query DB. Skill `/znf:explain-plan` và bước verify của `/znf:ship` chạy lệnh này tự động. Chạy tay khi bạn muốn xem trước.

## Kết quả

Lệnh không cần kết nối DB. Nó đọc diff giữa hai ref, tìm các query site và in từng finding kèm file, dòng, collection và gợi ý sửa. Finding BLOCKING phải xử lý hoặc waive trước khi ship. Finding ADVISORY chỉ để tham khảo.

Để waive một dòng, thêm chú thích `// znf:db-perf-ok: <lý do>` ngay trên dòng query đó. Finding chuyển sang WAIVED và lý do được ghi lại.

Khi diff không có query backend, lệnh in "gate pass". Khi không đọc được diff hoặc config, lệnh bỏ qua và không chặn.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--from` | Ref gốc của diff. Mặc định `origin/staging`. |
| `--to` | Ref đầu của diff. Mặc định `HEAD`. |
| `--diff-file` | Đọc diff từ file thay cho git. |
| `--json` | In kết quả dạng JSON. |

## Ví dụ

```bash
zenify db-perf --from origin/staging --to HEAD
```

## Lưu ý

Danh sách collection có tenant và các ngưỡng nằm trong file cấu hình của knowledge store. Một collection chưa được phân loại tạo ra finding BLOCKING cho tới khi bạn thêm nó vào danh sách.
