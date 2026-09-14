---
summary: Liệt kê worktree trong repo hiện tại, kèm branch, port, trạng thái merge và dev server đang chạy.
---
## Khi nào dùng

Khi bạn cần biết repo đang mở những task nào, task nào đã merge và có thể dọn, task nào còn dev server chạy.

## Kết quả

Bảng có các cột `SLUG`, `BRANCH`, `PORT`, `DEPS`, `MERGED`, `RUNNING`, `STALE`, `PATH`. Repo chưa có task in `wt: no tasks in this repo`. Cột `STALE` đánh `yes` cho mục còn ghi trong state nhưng thư mục đã mất; `zenify wt sweep` sẽ dọn mục đó.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--all` | Liệt kê mọi repo có cấu hình worktree trong workspace, gom theo repo. Repo không đọc được cấu hình bị bỏ qua kèm lý do. |
| `--json` | In mảng JSON thay cho bảng, để editor hoặc script đọc. |

## Ví dụ

```bash
zenify wt ls
zenify wt ls --all
zenify wt ls --json
```
