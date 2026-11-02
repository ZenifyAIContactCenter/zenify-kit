---
summary: In ra engine review mà kit sẽ dùng ở phiên này, agent hoặc codex, kèm lý do khi rơi về agent.
---
## Khi nào dùng

Khi muốn biết `znf:review` sẽ chạy bằng reviewer subagent của Claude hay bằng `codex review`. Lệnh chỉ đọc.

## Kết quả

Một dòng: `codex`, hoặc `agent (<lý do>)`. Kit chọn `codex` khi phiên đang chạy trong Codex (có `CODEX_THREAD_ID`) hoặc `ZNF_REVIEWER=codex`, và `codex` có trên PATH. Còn lại là `agent`; lý do là `ZNF_REVIEWER=agent` hoặc `codex không có trên PATH`.

## Ví dụ

```bash
zenify reviewer
zenify up --reviewer codex --apply
```

## Lưu ý

`ZNF_REVIEWER` nằm trong `env` của `~/.claude/settings.json`; `zenify up` seed giá trị `agent` khi chưa có và không đè giá trị bạn đã đặt. `zenify up --reviewer <agent|codex>` ghi đè giá trị đó khi chạy kèm `--apply`; [`zenify reviewer set`](/reference/cli/zenify_reviewer_set) ghi đè ngay.
