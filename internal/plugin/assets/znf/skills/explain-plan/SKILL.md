---
name: explain-plan
description: Use when a diff adds or changes a DB query — reads each query's plan and flags a missing-index scan (COLLSCAN / Seq Scan) on a large collection before it hits production. advisory only, never blocks.
allowed-tools: Read Grep Bash(db_read *)
---

# znf:explain-plan — soi query-plan, bắt COLLSCAN sớm

**Announce:** "Using znf:explain-plan to check query plans in this diff."

Một query thiếu index dùng được là **vô hình trên dữ liệu dev** (vài nghìn document, scan toàn
bộ vẫn nhanh) và chỉ cắn ở volume production. Skill này đọc plan của từng query trong diff và
báo `COLLSCAN` (Mongo) / `Seq Scan` (SQL) trên collection/table lớn. **Advisory:** báo findings,
KHÔNG chặn tiến độ. Fail-open — thiếu `db_read` hay không có query thì dừng sạch, không báo lỗi.

## Khi nào dùng

- Khi ground hoặc ship một thay đổi chạm DB (cook/ship/ground gọi ở đây).
- Hoặc gọi tay trên một diff bất kỳ có thêm/sửa query.

## Bước 1 — trigger cơ học (từ diff)

Đếm call-site query trong diff. Không có `db_read` trên PATH thì skill không áp dụng ở project này.

```bash
command -v db_read >/dev/null || echo "no db_read on PATH — skill này không áp dụng ở đây"
git diff HEAD | rg -c '\.find\(|\.aggregate\(|\.findOne\(|\.updateMany\(|\.skip\(|OFFSET|JOIN'
```

Non-zero → sang Bước 2. Zero → viết "no query in this diff" rồi dừng sạch.

## Bước 2 — chạy explain per-site

Với mỗi call-site: xác định collection/table **thật** (đừng đoán — liệt kê từ DB) và shape filter,
rồi chạy plan. Skill KHÔNG hardcode tên nào; mọi tên đến từ diff đang soi.

```bash
# Mongo
db_read eval 'db.getCollection("<real-name>").find({…}).explain("executionStats")'
# Quan hệ (MySQL/Postgres)
db_read sql 'EXPLAIN ANALYZE <câu-thật>'
```

Filter là biến/builder-chain (không dựng thẳng được) thì đọc code, dựng lại giá trị đại diện tay.

## Bước 3 — đọc plan theo rubric size-aware

| Plan thấy | Kết luận |
|---|---|
| `IXSCAN` / index được dùng | ok |
| `COLLSCAN` trên collection LỚN | FINDING |
| `Seq Scan` trên bảng LỚN | FINDING |

Lưu ý quan trọng: **index tồn tại ≠ được dùng**. Vẫn có thể quét toàn bộ dù đã có index khi:
sai thứ tự cột trong compound index · shape `$in` / `$or` · field không tuyển (non-selective).
Nên đọc `IXSCAN` thật trong plan, đừng suy ra từ "collection này có index".

## Bước 4 — báo cáo advisory

Mở đầu: "Advisory — không chặn tiến độ." Mỗi finding một dòng:

```
<file:line> · <collection/table> · verb quét (COLLSCAN/Seq Scan) · gợi ý index nên thêm
```

Không có finding → nói rõ đã soi N site, tất cả IXSCAN. Không block dù có finding.
