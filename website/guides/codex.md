---
title: Chạy với Codex
---

# Chạy với Codex

Kit chạy được trên cả Claude Code và Codex. Khi máy có `codex` trên PATH, `zenify up` tự nối kit vào Codex; khi không có, `zenify up` chỉ in một dòng `Codex: không cài, bỏ qua` và không tạo `~/.codex`. Phần Codex của kit nằm ở gốc workspace, không ở cấp user: Codex chỉ có kit khi mở ở gốc workspace.

## Cài và kiểm tra

Cài Codex theo hướng dẫn của OpenAI, rồi chạy `zenify up` như thường lệ. Cuối lần chạy, `up` in một khối `Codex:` gồm ba dòng: skills, hooks và agents, và nhắc bạn duyệt hook qua `/hooks`.

- **Skills:** Codex đọc cây skill của kit qua symlink `<workspace>/.agents/skills/znf`, trỏ về cùng cây skill Claude đang dùng (`~/.zenify/plugin-marketplace/plugins/znf/skills`); mỗi SessionStart làm mới cây này nếu nó cũ hơn binary.
- **Hooks:** nằm ở `<workspace>/.codex/hooks.json`, sinh từ cùng bảng hook với Claude (xem [Hook](/reference/hooks), cột Runtime).
- **Agents:** sinh từ các file agent của kit vào `<workspace>/.codex/agents`. File `.toml` mang marker của kit mà agent đã bị gỡ khỏi kit (ví dụ `architect.toml`) được `up` xoá và in một dòng `removed retired Codex agent …`.

- **Trust:** `up` thêm một mục trust của workspace vào `~/.codex/config.toml` (các mục khác và comment giữ nguyên), vì Codex chỉ nạp lớp `.codex/` của project khi đã trust. `zenify down` gỡ mục này. Khi `~/.codex/hooks.json` còn hook của kit (chưa chạy `up --apply` bản này, hoặc lần gỡ bản cấp user dừng giữa chừng), SessionStart không ghi hook Codex vào workspace nên cũng không thêm mục trust. `up` chỉ gỡ hook, agent và link skill Codex ở `~/` khi workspace đã có bản thay.

`zenify doctor` có các mục `codex-hooks`, `reviewer` và `agents-md` để kiểm tra phần Codex.

## Trust hook một lần

Codex bỏ qua hook chưa được trust mà không báo lỗi. Sau khi `zenify up` ghi hook mới hoặc đổi hook, mở `codex`, gõ `/hooks` và chọn trust. Đến lúc đó hook mới chạy.

## Gọi skill trên Codex

Gọi skill bằng `$znf:` cộng tên skill, ví dụ `$znf:fix` hoặc `$znf:ship`. Đối số là phần chữ đứng sau tên skill trong cùng tin nhắn; Codex không có `$ARGUMENTS`, nên các skill của kit không còn dùng placeholder đó. Subagent được tạo bằng tool subagent của Codex với cùng tên agent như trên Claude: `scout`, `code-reviewer`, `ui-verifier`, `researcher`, `spec-reviewer`.

## Dùng Codex làm reviewer

```bash
zenify up --reviewer codex
```

Lệnh này đặt `ZNF_REVIEWER=codex` trong `<workspace>/.claude/settings.local.json` (mặc định là `agent`); `zenify reviewer set|get` đọc ghi cùng file và báo lỗi khi chạy ngoài workspace. Chạy `zenify reviewer` để xem lựa chọn hiện tại.

## Codex còn thiếu gì

- Không có hook `read-guard`: Codex không có hook cho thao tác Read, nên hook này chỉ chạy trên Claude.
- Không có hook `write-guard`: Codex không có hook cho thao tác Write/Edit, nên hook này chỉ chạy trên Claude.
- Không có `claudeMdExcludes`.
