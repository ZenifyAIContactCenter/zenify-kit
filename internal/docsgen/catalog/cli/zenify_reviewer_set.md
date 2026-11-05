---
summary: Đặt engine review của kit, agent hoặc codex, vào `env.ZNF_REVIEWER` của `<workspace>/.claude/settings.local.json`.
---
## Khi nào dùng

Khi muốn đổi `znf:review` sang `codex review` hoặc về reviewer subagent của Claude mà không chạy lại `zenify up`. Dòng `Reviewer` của plugin `znf` trong `/config` của Claude Code gọi lệnh này khi bạn chọn `agent` / `codex`.

## Kết quả

Ghi `env.ZNF_REVIEWER` trong `<workspace>/.claude/settings.local.json` của workspace tìm được từ thư mục hiện tại, giữ nguyên mọi key khác, rồi in `ZNF_REVIEWER=<giá trị>`. Giá trị khác `agent` và `codex` thì lệnh báo lỗi, exit khác 0 và không đổi file. Chạy ngoài workspace lệnh cũng báo lỗi.

## Ví dụ

```bash
zenify reviewer set codex
zenify reviewer set agent
```

## Lưu ý

Session đang chạy đã đọc env lúc bắt đầu, nên giá trị mới có hiệu lực ở session mới. `zenify reviewer` in engine mà session hiện tại sẽ dùng.
