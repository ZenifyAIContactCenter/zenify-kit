---
summary: Tạo work item Plane giao cho chính bạn và in key kèm URL.
---
## Khi nào dùng

Khi có việc cần theo dõi trên Plane và bạn là người làm. Lệnh gọi `GET /users/me/`, tìm project theo identifier (mặc định `ZEN`), rồi tạo work item với `name`, `assignees` là chính bạn và `description_html` (từ `--description`, được escape). Việc mới hiện ở tab Backlog của pane ở nguồn `plane`.

## Kết quả

In một dòng `plane: <KEY>-<n> <url>`, ví dụ `plane: ZEN-308 https://plane.example/zenify/browse/ZEN-308/`. Không có PAT hoặc manifest không khai báo Plane thì lệnh thoát mã 2 và nhắc chạy `zenify up --apply`. Project không tồn tại thì lệnh thoát mã 1 và không tạo gì.

## Ví dụ

```bash
zenify plane create "Sửa lỗi export timeout"
zenify plane create "Dọn tài liệu" --project KB --description "Gộp hai trang trùng nội dung"
```
