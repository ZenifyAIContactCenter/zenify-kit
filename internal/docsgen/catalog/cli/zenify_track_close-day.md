---
summary: Chốt ngày làm việc trước, archive log cũ hơn 14 ngày.
---
## Khi nào dùng

Chạy tự động một lần mỗi ngày; chạy tay cũng được. Lần đầu trong ngày ghi `days/<ngày>.md`, chuyển dòng `log.md` và session log cũ sang `archive/`, rồi in báo cáo. Các lần sau cùng ngày không in gì. Lỗi giữa chừng thì không đánh dấu `day.closed`, lần sau chạy lại.

## Ví dụ

```bash
zenify track close-day
```
