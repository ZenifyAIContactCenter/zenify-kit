---
summary: Gắn hook git-guard vào ~/.claude/settings.json để chặn thao tác git vào branch deploy.
---
## Khi nào dùng

Thường bạn không cần chạy tay: `zenify up` và hook `SessionStart` tự gắn git-guard khi thiếu. Chạy lệnh này khi `zenify doctor` báo mục `git-guard` hỏng (hoặc dùng `zenify doctor --fix`), hay sau khi bạn sửa tay `~/.claude/settings.json`.

## Kết quả

Lệnh đăng ký hook `PreToolUse` với matcher `Bash` trỏ tới `zenify git-guard`, và gỡ mục cũ của phiên bản script nếu còn. Chạy lại khi đã có cấu hình, lệnh chỉ báo đã sẵn và không ghi gì.

## Ví dụ

```bash
zenify guard install
```

## Lưu ý

Danh sách branch deploy đọc từ `.claude/deploy-branches` của từng repo. Khi guard chặn, hãy báo lại thay vì tìm cách vòng qua.
