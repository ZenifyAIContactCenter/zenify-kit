---
summary: In ra engine review mà kit sẽ dùng ở phiên này, agent, codex hoặc project, kèm lý do khi rơi về agent.
---
## Khi nào dùng

Khi muốn biết `znf:review` sẽ chạy bằng reviewer subagent của Claude hay bằng `codex review`. Lệnh chỉ đọc.

## Kết quả

Một dòng: `project`, `codex`, hoặc `agent (<lý do>)`. Kit chọn `project` khi `ZNF_REVIEWER=project` và một `CLAUDE.md` (đi lên từ thư mục hiện tại, rồi `~/.claude/CLAUDE.md`) có dòng bắt đầu bằng `Pre-PR review:`; `project` thắng `CODEX_THREAD_ID`. `ZNF_REVIEWER=project` mà không có dòng đó thì rơi về `agent (không có dòng Pre-PR review)`. Kit chọn `codex` khi phiên đang chạy trong Codex (có `CODEX_THREAD_ID`) hoặc `ZNF_REVIEWER=codex`, và `codex` có trên PATH. Còn lại là `agent`; lý do là `ZNF_REVIEWER=agent` hoặc `codex không có trên PATH`.

## Ví dụ

```bash
zenify reviewer
zenify up --reviewer codex --apply
```

## Lưu ý

`ZNF_REVIEWER` nằm trong `env` của `<workspace>/.claude/settings.local.json`; `zenify up` seed giá trị `agent` khi chưa có và không đè giá trị bạn đã đặt. `zenify up --reviewer <agent|codex|project>` ghi đè giá trị đó khi chạy kèm `--apply`; [`zenify reviewer set`](/reference/cli/zenify_reviewer_set) ghi đè ngay.
