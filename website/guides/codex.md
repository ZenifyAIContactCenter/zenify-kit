---
title: Chạy với Codex
---

# Chạy với Codex

Kit chạy được trên cả Claude Code và Codex. Khi máy có `codex` trên PATH, `zenify up` tự nối kit vào Codex; khi không có, `zenify up` chỉ in một dòng `Codex: không cài, bỏ qua` và không tạo `~/.codex`.

## Cài và kiểm tra

Cài Codex theo hướng dẫn của OpenAI, rồi chạy `zenify up` như thường lệ. Cuối lần chạy, `up` in một khối `Codex:` gồm bốn dòng: skills, hooks, agents và MCP playwright.

- **Skills:** Codex đọc cây skill của kit qua một symlink ở `~/.agents/skills`, trỏ về cùng cây skill Claude đang dùng.
- **Hooks:** nằm ở `~/.codex/hooks.json`, sinh từ cùng bảng hook với Claude (xem [Hook](/reference/hooks), cột Runtime).
- **Agents:** sinh từ các file agent của kit. File `.toml` mang marker của kit mà agent đã bị gỡ khỏi kit (ví dụ `architect.toml`) được `up` xoá và in một dòng `removed retired Codex agent …`.

`zenify doctor` có các mục `codex-hooks`, `reviewer` và `agents-md` để kiểm tra phần Codex.

## Trust hook một lần

Codex bỏ qua hook chưa được trust mà không báo lỗi. Sau khi `zenify up` ghi hook mới hoặc đổi hook, mở `codex`, gõ `/hooks` và chọn trust. Đến lúc đó hook mới chạy.

## Gọi skill trên Codex

Gọi skill bằng `$znf:` cộng tên skill, ví dụ `$znf:fix` hoặc `$znf:ship`. Đối số là phần chữ đứng sau tên skill trong cùng tin nhắn; Codex không có `$ARGUMENTS`, nên các skill của kit không còn dùng placeholder đó. Subagent được tạo bằng tool subagent của Codex với cùng tên agent như trên Claude: `scout`, `code-reviewer`, `ui-verifier`, `researcher`, `spec-reviewer`.

## Dùng Codex làm reviewer

```bash
zenify up --reviewer codex
```

Lệnh này đặt `ZNF_REVIEWER=codex` (mặc định là `agent`). Chạy `zenify reviewer` để xem lựa chọn hiện tại.

## Codex còn thiếu gì

- Không có hook `read-guard`: Codex không có hook cho thao tác Read, nên hook này chỉ chạy trên Claude.
- Không có hook `write-guard`: Codex không có hook cho thao tác Write/Edit, nên hook này chỉ chạy trên Claude.
- Không có `claudeMdExcludes`.
