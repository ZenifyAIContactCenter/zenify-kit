---
title: Đọc dữ liệu thật
---

# Đọc dữ liệu thật

`zenify db-read` đọc database dùng chung ở chế độ chỉ đọc, để bạn xác nhận tên collection, bảng và field trước khi viết code chạm vào chúng.

## Khi nào dùng

Dùng lệnh này trước khi viết code chạm một collection, bảng hoặc field. Tên collection và field phải lấy từ dữ liệu thật, không đoán từ tên model trong code. Skill [`/znf:ground`](/reference/skills/ground) gọi lệnh này thay bạn khi grounding một field hoặc một shape dữ liệu.

## Lệnh

```bash
zenify db-read <collections|tables|doc|count|find|aggregate|explain|sql> [arg]
```

| Lệnh con | Kết quả |
|---|---|
| `collections [chuỗi]` | Liệt kê collection Mongo, lọc theo chuỗi con nếu có |
| `tables [chuỗi]` | Liệt kê bảng MySQL, lọc theo chuỗi con nếu có |
| `doc <collection>` | In một document thật để xem tên field |
| `count <collection>` | Đếm document, thêm `--filter` để đếm có điều kiện |
| `find <collection>` | Tìm document với `--filter`, `--projection`, `--sort`, `--limit` |
| `aggregate <collection>` | Chạy `--pipeline`, từ chối `$out` và `$merge` |
| `explain <collection>` | In executionStats cho `--filter`/`--sort` hoặc `--pipeline`, không áp `--limit` (chạy truy vấn đọc, tối đa 30 giây) |
| `sql '<câu lệnh>'` | Chạy một câu SQL chỉ đọc trong transaction READ ONLY |

Không lệnh nào nhận code tự do. Lệnh `eval` đã bị gỡ; muốn liệt kê các giá trị của một field (thay cho `distinct`), dùng `aggregate`. Danh sách bị giới hạn bởi `--limit` (tối đa 200); có thông báo trên stderr nghĩa là danh sách đã bị cắt:

```bash
zenify db-read aggregate chat_rooms --pipeline '[{"$group":{"_id":"$status"}}]'
zenify db-read find chat_rooms --filter '{"_id":{"$oid":"<id>"}}' --limit 5
```

```bash
zenify db-read collections chatbot
zenify db-read doc chat_rooms
zenify db-read sql 'DESCRIBE agents'
```

## Kết quả

Kết nối lấy từ biến môi trường `MONGO_URL` và các biến `MYSQL_*` trong khối `env` của file `.claude/settings.local.json` ở workspace. Lệnh không nhận chuỗi kết nối qua tham số dòng lệnh và không in ra màn hình, nên chạy `zenify db-read` không bao giờ làm lộ credential.

`--limit` mặc định 50 và tối đa 200. Một truy vấn chạy tối đa 30 giây, cả tiến trình tối đa 60 giây. Filter Mongo nhận Extended JSON, ví dụ `{"_id":{"$oid":"..."}}`.

Khi kết quả nhiều hơn `--limit`, lệnh in đủ `--limit` dòng rồi ghi thông báo `… đã cắt ở <limit> dòng (--limit, tối đa 200)` ra stderr, nên stdout chỉ chứa dữ liệu. `explain` không áp `--limit`. `--pipeline` không dùng chung với `--filter`, `--projection` hay `--sort`, và `find`, `count` không nhận `--pipeline`. `doc`, `find` và `aggregate` in Extended JSON, ví dụ `{"$oid":"..."}` và `{"$date":"..."}`.

`sql` chỉ nhận một câu `SELECT`, `WITH`, `SHOW`, `DESCRIBE`, `DESC` hoặc `EXPLAIN`, chạy trong transaction READ ONLY. Ngoài chuỗi literal, lệnh từ chối comment, ký tự `\`, các từ `DELETE`, `UPDATE`, `INSERT`, `INTO`, `DELIMITER`, `CHARSET`, `FOR SHARE` và `LOCK IN SHARE MODE`. Khi gặp, lệnh báo "refused" và dừng, không thực thi.

## Lưu ý

- Lệnh chỉ chạy được trong phiên mở tại workspace, vì biến môi trường đến từ settings của workspace, không phải từ `.env` riêng của một repo.
- Tên collection trong database luôn ở số nhiều. Tên số ít bạn thấy trong code là tên model, không phải tên collection thật. Luôn liệt kê bằng `zenify db-read collections` trước khi dùng một tên, đừng lấy tên từ tài liệu hay từ một document mẫu.
- Khi kết nối lỗi, lệnh gợi ý cách tách lỗi mạng khỏi lỗi xác thực thay vì chỉ báo timeout.
- `/znf:ground` dùng lệnh này để xác minh field trước khi code chạm vào nó. `/znf:ship` cũng đọc dữ liệu thật qua đường này khi verify một thay đổi chạm DB.

## Secret và settings.local.json

Secret dùng để đọc dữ liệu thật, ví dụ chuỗi kết nối DB hay tài khoản đăng nhập test, sống trong khối `env` của file `.claude/settings.local.json` ở workspace. File này không được commit.

Các tool như `zenify db-read` đọc secret từ biến môi trường tại thời điểm chạy, chứ không nhận qua tham số dòng lệnh. Nhờ vậy không credential nào lọt vào lịch sử lệnh, log, hay màn hình. `zenify db-read` chỉ nhận các lệnh có cấu trúc cố định, từ chối in ra chuỗi kết nối và từ chối mọi câu lệnh mang tính ghi ngay từ thiết kế, không phải nhờ quyền hạn tài khoản bị giới hạn.

Cùng nguyên tắc đó áp dụng khi bạn tự viết script hay agent chạm secret: không bao giờ ghi một mật khẩu ở dạng chữ thẳng vào file, kể cả file tạm.

`zenify wt new` seed `.claude/settings.local.json` từ checkout chính vào mỗi worktree mới, qua danh sách `copy` khai trong `.claude/worktree.json`. Nhờ vậy một worktree dùng chung đúng bộ secret và đúng bộ nhớ với checkout chính, thay vì mỗi worktree tự ghi vào một bộ nhớ riêng biệt. Xem [Onboard workspace và repo](/guides/onboard-workspace) để biết `copy` còn seed những gì khác.

## Xem thêm

[`zenify db-read`](/reference/cli/zenify_db-read), [`/znf:ground`](/reference/skills/ground), [Knowledge store](/concepts/knowledge-store), [Onboard workspace và repo](/guides/onboard-workspace)

<!-- Nguồn (cho người bảo trì, không hiển thị):
- reference/cli/zenify_db-read.md (generated)
- reference/skills/ground.md (generated)
- reference/cli/zenify_wt_new.md, concepts/base-ref-and-ports.md (copy seeding settings.local.json into worktree)
-->
