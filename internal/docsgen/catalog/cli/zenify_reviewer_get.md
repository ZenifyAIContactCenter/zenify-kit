---
summary: In engine review đã lưu trong `env.ZNF_REVIEWER` của `~/.claude/settings.json`, agent hoặc codex.
---
## Khi nào dùng

Khi cần biết giá trị mà `zenify reviewer set` đã ghi, không phụ thuộc session đang chạy. Dòng `Reviewer` của plugin `znf` trong `/config` của Claude Code đọc lệnh này mỗi lần mở session để kéo dòng về đúng nguồn.

## Kết quả

In `agent` hoặc `codex`. File hoặc key chưa có thì in `agent` (mặc định). Giá trị khác `agent` và `codex` thì lệnh báo lỗi và exit khác 0.

## Ví dụ

```bash
zenify reviewer get
```

## Lưu ý

`zenify reviewer` in engine mà session hiện tại sẽ dùng (theo env của session và việc `codex` có trên PATH hay không), nên có thể khác giá trị đã lưu.
