---
summary: Ghi hook, model và env của kit vào máy và workspace mà không cần zenify up.
---
## Khi nào dùng

Trên máy không chạy `zenify up` và không có manifest workspace, ví dụ container worker của zenie. Lệnh chạy đúng bước mà `zenify up` và hook SessionStart chạy: nối hook vào `~/.claude/settings.json`, đặt `CLAUDE_CODE_SUBAGENT_MODEL`, ghim model phiên chính vào `.claude/settings.json` của workspace. Nhờ vậy máy đó cấu hình giống máy của người trong team dùng cùng bản kit.

## Kết quả

Chỉ in một dòng cho mỗi thứ vừa thay đổi; lần chạy lại không in gì. Bước nào lỗi thì in một dòng `znf ensure:` ra stderr rồi chạy bước tiếp theo.

## Ví dụ

```bash
zenify ensure --workspace /work
```
