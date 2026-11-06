---
title: zenify db-read
---

[Lệnh CLI](/reference/cli/) / `zenify db-read`

# `zenify db-read`

Đọc database dùng chung ở chế độ chỉ đọc, không bao giờ ghi và không in secret.

## Cú pháp

```text
zenify db-read <collections|tables|doc|count|find|aggregate|explain|indexes|stats|sql> [arg] [flags]
```

## Khi nào dùng

Trước khi viết code chạm một collection, field hoặc bảng. Tên collection và field lấy từ dữ liệu thật, không đoán từ tên model. Skill `/znf:ground` dùng lệnh này.

## Kết quả

Mười lệnh con:

| Lệnh con | Kết quả |
|---|---|
| `collections [chuỗi]` | Liệt kê collection Mongo, lọc theo chuỗi con nếu có |
| `tables [chuỗi]` | Liệt kê bảng MySQL, lọc theo chuỗi con nếu có |
| `doc <collection>` | In một document thật để xem tên field |
| `count <collection>` | Đếm document, thêm `--filter` để đếm có điều kiện |
| `find <collection>` | Tìm document với `--filter`, `--projection`, `--sort`, `--limit` |
| `aggregate <collection>` | Chạy `--pipeline`, từ chối `$out` và `$merge` |
| `explain <collection>` | In executionStats cho `--filter`/`--sort` hoặc `--pipeline`, không áp `--limit` (chạy truy vấn đọc, tối đa 30 giây) |
| `indexes <collection>` | In `getIndexes()` của collection |
| `stats <collection>` | In kích thước, số document và số index của collection (`count`, `size`, `avgObjSize`, `storageSize`, `nindexes`, `totalIndexSize`), không có nội dung document |
| `sql '<câu lệnh>'` | Chạy một câu SQL chỉ đọc trong transaction READ ONLY |

Khi manifest khai báo `broker:`, lệnh gửi truy vấn qua credential broker, nên bạn không giữ chuỗi kết nối nào; `--prd` đọc production. Khi manifest không có `broker:`, kết nối lấy từ `MONGO_URL` và `MYSQL_URL` (`mysql://user:pass@host:port/database`, ký tự `@ : / # ? %` và dấu cách trong user hoặc mật khẩu phải percent-encode, ví dụ `%` thành `%25`, dấu cách thành `%20`) trong `<workspace>/.zenify/.env` (xem `zenify env`), và `--prd` dùng `MONGO_URL_PRD`, `MYSQL_URL_PRD`. Mật khẩu MySQL chỉ đi qua biến `MYSQL_PWD` của tiến trình con. Lệnh không nhận chuỗi kết nối qua tham số và không in ra màn hình.

Không lệnh nào nhận code tự do. `--limit` mặc định 50, tối đa 200. Một truy vấn chạy tối đa 30 giây, cả tiến trình tối đa 60 giây. Filter Mongo nhận Extended JSON, ví dụ `{"_id":{"$oid":"..."}}`. Thay cho `distinct`: `zenify db-read aggregate <collection> --pipeline '[{"$group":{"_id":"$field"}}]'`.

Khi kết quả nhiều hơn `--limit`, lệnh in đủ `--limit` dòng rồi ghi thông báo `… đã cắt ở <limit> dòng (--limit, tối đa 200)` ra stderr, nên stdout chỉ chứa dữ liệu. Stdout của mọi lệnh đọc tối đa 1 MiB: quá mức này lệnh dừng ở ranh giới dòng gần nhất rồi ghi `… đã cắt ở 1048576 byte (giới hạn output)` ra stderr. `explain` không áp `--limit`. `--pipeline` không dùng chung với `--filter`, `--projection` hay `--sort`, và `find`, `count` không nhận `--pipeline`. Lệnh từ chối flag mà lệnh con không dùng: `collections`, `tables`, `doc`, `indexes`, `stats`, `sql` không nhận `--filter`, `--projection`, `--sort`, `--pipeline`; `count` không nhận `--projection`, `--sort`. `doc`, `find` và `aggregate` in Extended JSON, ví dụ `{"$oid":"..."}` và `{"$date":"..."}`.

`sql` chỉ nhận một câu `SELECT`, `WITH`, `SHOW`, `DESCRIBE`, `DESC` hoặc `EXPLAIN`. Ngoài chuỗi literal, lệnh từ chối comment, ký tự `\`, các từ `DELETE`, `UPDATE`, `INSERT`, `INTO`, `DELIMITER`, `FOR SHARE` và `LOCK IN SHARE MODE`, và báo "refused" rồi dừng. Ký tự xuống dòng nằm trong chuỗi literal cũng bị từ chối; dùng `CHAR(10)` thay thế, xuống dòng ngoài chuỗi vẫn dùng được.

## Ví dụ

```bash
zenify db-read collections chatbot
zenify db-read doc chat_rooms
zenify db-read sql 'DESCRIBE agents'
```

## Lưu ý

Manifest quyết định đường chạy là manifest nhúng sẵn trong binary, hoặc file bạn chỉ bằng `--manifest`; lệnh không bao giờ đọc `manifest/repos.yaml` ở thư mục hiện tại. Một `--manifest` không tồn tại hoặc sai là lỗi, lệnh không lùi về credential trên máy. Manifest đó khai báo `broker:` thì lệnh kiểm tra tham số ở máy bạn như cũ rồi gửi truy vấn qua credential broker, nên không cần `.zenify/.env`; `--prd` in thêm dòng `zenify db-read: production (via broker)` ra stderr. Không có `broker:` thì lệnh chạy tại máy như trước. `--overlay` chỉ đổi đường dẫn repo, không đổi `broker:`.

Qua broker, dev (team `zenify-team`) đọc production chỉ được `collections`, `count` không có `--filter`, `explain` (chỉ plan `queryPlanner`, không chạy truy vấn), `indexes`, `stats`, `tables` và `sql` dạng `EXPLAIN [FORMAT=…] SELECT …` (đúng một `SELECT`, không subquery, không `ANALYZE`) hoặc `DESCRIBE <bảng>`/`SHOW INDEX`/`SHOW CREATE TABLE`/`SHOW TABLE STATUS`/`SHOW TABLES`/`SHOW COLUMNS` không có `WHERE`, `(` hay `SELECT`; lệnh khác báo `lệnh này chỉ dành cho lead (team leaders); dev trên prd chỉ dùng db-read: collections, count (no --filter), explain, indexes, stats, tables, sql: EXPLAIN SELECT without subquery, DESCRIBE <table>, SHOW INDEX|COLUMNS|TABLES|CREATE TABLE|TABLE STATUS; redis-read: EXISTS LLEN SCAN SCARD TTL TYPE ZCARD`. `--target audit` đọc database audit log (chỉ Mongo, chỉ production, chỉ lead); `--target lumi` đọc MySQL của lumi-agent (`db_chatbot` trên production, `mass_chatbot_v2` trên staging), chỉ `sql` và `tables`, trên production chỉ lead; `--target msghub` đọc Mongo của message-hub, mọi lệnh Mongo, trên production chỉ lead; qua broker, lỗi kết nối không gợi ý `.zenify/.env` hay tên biến `MONGO_URL_PRD`. Staging không giới hạn theo role. Xem [Dùng credential broker](/guides/broker-client#lead-dev).

Không có broker thì lệnh chỉ chạy được trong phiên mở tại workspace, vì credential nằm ở `.zenify/.env` của workspace. Khi kết nối lỗi, lệnh gợi ý cách tách lỗi mạng khỏi lỗi xác thực.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--filter` | JSON object filter (find, count, explain) |
| `--limit` | max documents or rows, 1..200 (mặc định `50`) |
| `--manifest` | path to the repos manifest whose broker: decides the route (default: the manifest embedded in the binary, never cwd) |
| `--overlay` | path to a personal overlay (default: none; an overlay never changes broker:) |
| `--pipeline` | JSON array pipeline (aggregate, explain) |
| `--prd` | read production: via the broker; without one: MONGO_URL_PRD / MYSQL_URL_PRD |
| `--projection` | JSON object projection (find, explain) |
| `--sort` | JSON object sort (find, explain) |
| `--target` | database: main (default), audit (Mongo, prd only), lumi (lumi-agent MySQL) or msghub (message-hub Mongo) |

## Lệnh liên quan

- [zenify](./zenify): Binary dòng lệnh của ZenifyKit, chứa mọi lệnh workspace và plugin skill đi kèm.

<!-- Generated by zenify docs gen: prose from internal/docsgen/catalog, facts from the command tree. -->
