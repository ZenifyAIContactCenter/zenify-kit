---
summary: Nhóm lệnh quản lý worktree theo slug: tạo, liệt kê, gỡ, dọn, kèm port và môi trường dev riêng cho từng task.
---
## Khi nào dùng

Mỗi task sửa code nằm trong một worktree riêng. Nhóm `zenify wt` tạo worktree đó từ base ref đã khai trong `.claude/worktree.json`, cấp port, seed file env, rồi dọn khi branch đã merge. Các skill của kit gọi nhóm lệnh này thay bạn, nhưng bạn vẫn có thể gõ tay.

Mọi lệnh trong nhóm chạy từ bên trong repo, hoặc bên trong một worktree của repo đó. Xem [Worktree theo slug](/concepts/worktree-per-slug) để hiểu vòng đời một slug.
