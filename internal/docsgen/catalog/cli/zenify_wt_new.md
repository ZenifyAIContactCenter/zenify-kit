---
summary: Tạo worktree mới cho một task, kèm branch, port riêng và môi trường dev đã seed.
---
## Khi nào dùng

Trước lần sửa code đầu tiên của một task. Mọi thay đổi code nằm trong worktree, checkout chính chỉ để đọc và chạy.

## Kết quả

Lệnh tạo worktree trong `.worktrees/<slug>`, checkout branch `<user>/<type>/<slug>` từ base ref đã khai trong `.claude/worktree.json`, cấp một port trong dải của repo, copy các file khai ở `copy` và cài deps theo `deps`. Lệnh in đường dẫn worktree và port.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--type` | Loại task: `feat`, `fix`, `chore` hoặc `hotfix`. Mặc định `feat`. `hotfix` đổi base ref sang `hotfixBaseRef`. |
| `--base` | Base ref tự chọn, thắng mọi giá trị trong `worktree.json`. |
| `--another` | Cho phép mở task thứ hai trong repo khi đã có một worktree chưa merge. |
| `--install` | Buộc cài deps mới dù config khai `symlink` hoặc `clone`. |

## Ví dụ

```bash
git fetch origin
zenify wt new session-timeout --type fix
```

## Lưu ý

Lệnh từ chối khi repo đã có worktree chưa merge của bạn và in lệnh `cd` vào worktree đó. Dùng `--another` chỉ khi đó thực sự là việc khác. Xem thêm [Mỗi slug một worktree](/concepts/worktree-per-slug).
