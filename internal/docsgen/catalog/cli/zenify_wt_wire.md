---
summary: Trỏ file env của worktree hiện tại sang dev server của các service liên quan đang được sửa cùng slug.
---
## Khi nào dùng

Khi một task sửa nhiều repo cùng lúc. Worktree ở repo A cần gọi sang worktree ở repo B thay cho bản đang chạy từ checkout chính.

## Kết quả

Với mỗi biến khai trong mục `peers` của `.claude/worktree.json`, lệnh ghi lại giá trị: nếu repo peer có worktree cùng slug, dùng port của worktree đó; nếu không, dùng giá trị trong file env của checkout chính. Lệnh chạy nhiều lần cho cùng kết quả, và chạy lại sau khi peer đã gỡ sẽ trả về giá trị gốc.

Lệnh chỉ chạy từ bên trong worktree. Repo không khai `peers` in `wt: this repo declares no peers — nothing to wire`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `-n`, `--dry-run` | Chỉ in các dòng sẽ đổi, không ghi file. |

## Ví dụ

```bash
zenify wt wire --dry-run
zenify wt wire
```

## Lưu ý

`zenify wt new` tự gọi wire khi tạo worktree. Nếu file env được git theo dõi, wire làm branch bẩn và lệnh cảnh báo.
