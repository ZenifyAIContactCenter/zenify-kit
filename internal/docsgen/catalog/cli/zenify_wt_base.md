---
summary: In base thật của worktree hiện tại, để skill tính merge-base đúng.
---
## Khi nào dùng

Khi cần merge-base của worktree với base mà nó đã tách ra. Một worktree có thể tách từ một feature branch thay vì `baseRef` cố định của repo, nên skill gọi lệnh này thay vì tự đọc `baseRef`.

## Kết quả

Lệnh in đúng một dòng là ref base, theo thứ tự ưu tiên: `wt.base` do `wt new` ghi lúc tạo worktree, rồi `baseRef` trong `.claude/worktree.json`, rồi `origin/main`. Chạy trong thư mục của worktree.

## Ví dụ

```bash
git merge-base "$(zenify wt base)" HEAD
```
