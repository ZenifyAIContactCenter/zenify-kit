---
summary: Thêm các mục vào kế hoạch hôm nay và in `track: plan <n> items`.
---
## Khi nào dùng

Giữa ngày, khi có việc mới cần đưa vào kế hoạch mà không muốn gõ lại cả danh sách. Mỗi ref là mã ngắn trên pane (ví dụ `hub#3`), id backlog, id SDD, id track hoặc URL issue/PR; mã ngắn được tìm trong kế hoạch, backlog và việc đang chạy. Mục đã có trong kế hoạch không bị thêm lần hai. Lệnh ghi một dòng `plan` mới chứa toàn bộ danh sách.

## Ví dụ

```bash
zenify track plan add hub#3 https://github.com/o/r/pull/3
```
