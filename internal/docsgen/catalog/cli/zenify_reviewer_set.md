---
summary: Đặt engine review của kit, agent hoặc codex, vào `env.ZNF_REVIEWER` của `~/.claude/settings.json`.
---
## Khi nào dùng

Khi muốn đổi `znf:review` sang `codex review` hoặc về reviewer subagent của Claude mà không chạy lại `zenify up`. Pane `/znf-settings` (tab `Reviewer`) gọi lệnh này qua hai nút `agent` / `codex`.

## Kết quả

Ghi `env.ZNF_REVIEWER` trong `~/.claude/settings.json`, giữ nguyên mọi key khác, rồi in `ZNF_REVIEWER=<giá trị>`. Giá trị khác `agent` và `codex` thì lệnh báo lỗi, exit khác 0 và không đổi file.

## Ví dụ

```bash
zenify reviewer set codex
zenify reviewer set agent
```

## Lưu ý

Session đang chạy đã đọc env lúc bắt đầu, nên giá trị mới có hiệu lực ở session mới. `zenify reviewer` in engine mà session hiện tại sẽ dùng.
