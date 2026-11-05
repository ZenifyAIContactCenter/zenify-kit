---
summary: Ghi kế hoạch hôm nay từ các id đã cho và in `track: plan <n> items`.
---
## Khi nào dùng

Đầu ngày hoặc khi đổi kế hoạch. Mỗi id là bid backlog, id SDD (`repos/<...>/progress.md:<dòng>`), id track hoặc URL issue/PR; id không hợp lệ làm lệnh lỗi và không ghi gì. Id trùng chỉ tính một lần; lần `set` sau thay kế hoạch trước.

## Ví dụ

```bash
zenify track plan set b-261105-ab12 https://github.com/o/r/pull/3
```
