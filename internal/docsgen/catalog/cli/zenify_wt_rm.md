---
summary: Gỡ một worktree đã xong việc, từ chối khi còn thay đổi chưa commit hoặc branch chưa merge.
---
## Khi nào dùng

Khi một task đã merge và bạn muốn dọn worktree cùng branch của nó. Để dọn mọi task đã merge một lượt, dùng `zenify wt sweep`.

## Kết quả

Lệnh gỡ worktree khỏi git, xóa branch cục bộ và cập nhật state của repo. Trước đó nó kiểm tra và từ chối trong ba trường hợp, mỗi trường hợp một thông điệp riêng:

- còn thay đổi chưa commit,
- đang ở detached HEAD,
- branch chưa có dấu vết merge trong base.

Bạn có thể gọi thử mà không sợ mất việc chưa land.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `-f`, `--force` | Gỡ kể cả khi dirty, detached hoặc chưa merge. Dùng khi branch đã squash-merge nên không còn dấu vết merge. |

## Ví dụ

```bash
zenify wt rm session-timeout
zenify wt rm session-timeout --force
```
