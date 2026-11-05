---
summary: Ghi hook, model và env của kit vào máy và workspace mà không cần zenify up.
---
## Khi nào dùng

Trên máy không chạy `zenify up` và không có manifest workspace, ví dụ container worker của zenie. Lệnh chạy đúng bước mà `zenify up` và hook SessionStart chạy: cập nhật plugin local khi bản cũ hơn binary, nối hook, `git-guard`, `CLAUDE_CODE_SUBAGENT_MODEL` và luật của kit vào `<workspace>/.claude/settings.local.json` (cùng file Codex trong `<workspace>/.codex` khi có `codex`), ghim model phiên chính vào `.claude/settings.json` của workspace, và tạo file đánh dấu workspace `.zenify/manifest.json` khi chưa có (file đã có thì không bao giờ ghi đè), để `zenify wt` và các lệnh khác nhận ra thư mục đó là workspace. Nhờ vậy máy đó cấu hình giống máy của người trong team dùng cùng bản kit.

## Kết quả

Trên máy chưa chuyển sang cài ở workspace, SessionStart và `zenify ensure` xét từng phần theo bản cấp user của chính nó, để hook không chạy hai lần: `~/.claude/settings.json` còn hook của kit thì không ghi hook, env hay permissions; còn `git-guard` cấp user thì không ghi `git-guard`; `~/.codex/hooks.json` còn hook của kit thì không ghi hook Codex và mục trust. Lệnh in `kit vẫn cài ở cấp user; chạy zenify up --apply để chuyển sang workspace`. Chạy `zenify up --apply` để chuyển.

Chỉ in một dòng cho mỗi thứ vừa thay đổi; lần chạy lại không in gì. Bước nào lỗi thì in một dòng `znf ensure:` ra stderr rồi chạy bước tiếp theo.

## Ví dụ

```bash
zenify ensure --workspace /work
```
