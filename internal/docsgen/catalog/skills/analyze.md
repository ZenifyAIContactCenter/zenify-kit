---
summary: Kiểm tra cặp spec và plan trước khi code: độ phủ FR sang task, marker chưa làm rõ, cấu trúc Brief và bốn tiêu chí chất lượng.
---
## Khi nào dùng

Sau khi spec và plan đều đã viết xong, trước khi giao cho implementer. `/znf:cook` gọi skill này ở đúng thời điểm đó. Bạn cũng có thể gọi tay trên bất kỳ cặp spec và plan nào.

## Cách hoạt động

1. Skill chạy `zenify analyze --spec <spec> --plan <plan>` để lấy phần kiểm tra cơ học: FR không có task nào phủ (CRITICAL), task không khai `_Requirements:` (HIGH), plan trích FR không có trong spec (HIGH), marker `[NEEDS CLARIFICATION` còn sót (HIGH), Brief có đủ 8 field không, ba tag `_Blast-radius:`, `_DB:`, `_Rollback:` có mặt không.
2. Skill đọc thêm bằng phán đoán, mỗi phát hiện ở mức MEDIUM: SC có kiểm được không, Brief có giải thích vì sao đường hiện có không đủ, khối DB guarantees có thật hay chỉ ghi chung, ba tag rủi ro có nội dung thật không.
3. Báo cáo gộp hai phần, sắp theo CRITICAL, HIGH, MEDIUM, mở đầu bằng câu "Advisory — does not block progress." Skill không có quyền chặn; bạn hoặc `/znf:cook` quyết định sửa spec hay đi tiếp.

## Ví dụ

```text
/znf:analyze docs/specs/contact-center-be/2026-09-14-ticket-tags-design.md docs/plans/contact-center-be/2026-09-14-ticket-tags.md
```

## Lưu ý

Lệnh `zenify analyze` fail-open. Khi lệnh báo không phân tích được, skill ghi nhận và đi tiếp thay vì coi là lỗi chặn.
