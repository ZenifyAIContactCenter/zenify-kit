---
summary: Kết thúc một branch sau khi test xanh: push và mở PR, hoặc giữ nguyên; skill không bao giờ merge.
---
## Khi nào dùng

Khi implement xong và test đã pass, cần quyết định tích hợp công việc. `/znf:subagent-driven-development` gọi skill này ở bước cuối.

## Cách hoạt động

1. Skill chạy toàn bộ test suite của project bằng runner của project (với Node là `pm test`, không mặc định `npm`). Test fail thì báo danh sách fail và dừng.
2. Skill nhận diện môi trường: repo thường, worktree có tên branch, hay detached HEAD; xác nhận base branch với bạn nếu chưa rõ.
3. Skill trình đúng hai lựa chọn: push và mở PR, hoặc giữ nguyên branch. Không có lựa chọn merge: merge là quyết định deploy của bạn. Skill chờ bạn chọn.
4. Cả hai lựa chọn đều giữ worktree. Chỉ khi bạn yêu cầu bỏ công việc, skill mới dọn worktree dưới `.worktrees/` bằng `git worktree remove` không có `--force`; khi bị từ chối vì còn file chưa commit, skill liệt kê và hỏi bạn xử lý.

## Lưu ý

Xóa công việc chỉ xảy ra khi bạn yêu cầu và gõ đúng chữ `discard`. Trước khi hiện menu, skill ghi session-handoff nếu phiên có điều đáng tiếp tục.
