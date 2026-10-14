---
summary: Đọc database dùng chung ở chế độ chỉ đọc, không bao giờ ghi và không in secret.
---
## Khi nào dùng

Trước khi viết code chạm một collection, field hoặc bảng. Tên collection và field lấy từ dữ liệu thật, không đoán từ tên model. Skill `/znf:ground` dùng lệnh này.

## Kết quả

Tám lệnh con:

| Lệnh con | Kết quả |
|---|---|
| `collections [chuỗi]` | Liệt kê collection Mongo, lọc theo chuỗi con nếu có |
| `tables [chuỗi]` | Liệt kê bảng MySQL, lọc theo chuỗi con nếu có |
| `doc <collection>` | In một document thật để xem tên field |
| `count <collection>` | Đếm document, thêm `--filter` để đếm có điều kiện |
| `find <collection>` | Tìm document với `--filter`, `--projection`, `--sort`, `--limit` |
| `aggregate <collection>` | Chạy `--pipeline`, từ chối `$out` và `$merge` |
| `explain <collection>` | In queryPlanner cho `--filter`/`--sort` hoặc `--pipeline` |
| `sql '<câu lệnh>'` | Chạy một câu SQL chỉ đọc trong transaction READ ONLY |

Kết nối lấy từ biến môi trường `MONGO_URL` và các biến `MYSQL_*` trong khối `env` của file `.claude/settings.local.json` ở workspace. Lệnh không nhận chuỗi kết nối qua tham số và không in ra màn hình.

Không lệnh nào nhận code tự do. `--limit` mặc định 50, tối đa 200. Một truy vấn chạy tối đa 30 giây, cả tiến trình tối đa 60 giây. Filter Mongo nhận Extended JSON, ví dụ `{"_id":{"$oid":"..."}}`. Thay cho `distinct`: `zenify db-read aggregate <collection> --pipeline '[{"$group":{"_id":"$field"}}]'`.

`sql` chỉ nhận một câu `SELECT`, `WITH`, `SHOW`, `DESCRIBE`, `DESC` hoặc `EXPLAIN`. Ngoài chuỗi literal, lệnh từ chối comment, ký tự `\`, các từ `DELETE`, `UPDATE`, `INSERT`, `INTO`, `FOR SHARE` và `LOCK IN SHARE MODE`, và báo "refused" rồi dừng.

## Ví dụ

```bash
zenify db-read collections chatbot
zenify db-read doc chat_rooms
zenify db-read sql 'DESCRIBE agents'
```

## Lưu ý

Chỉ chạy được trong phiên mở tại workspace, vì biến môi trường đến từ settings của workspace. Khi kết nối lỗi, lệnh gợi ý cách tách lỗi mạng khỏi lỗi xác thực.
