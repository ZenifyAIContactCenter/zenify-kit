---
summary: In ra engine review mà kit sẽ dùng ở phiên này, agent, codex, project hoặc agent+project, kèm lý do khi rơi về agent.
---
## Khi nào dùng

Khi muốn biết `znf:review` sẽ chạy bằng reviewer subagent của Claude, bằng `codex review`, bằng reviewer mà project khai báo, hay cả Claude lẫn reviewer của project. Lệnh chỉ đọc.

## Kết quả

Một dòng: `agent+project`, `project`, `codex`, hoặc `agent (<lý do>)`. `ZNF_REVIEWER` chưa đặt được coi là mặc định `agent+project`. Kit chọn `agent+project` (hoặc `project`) khi `ZNF_REVIEWER` mang đúng giá trị đó và một `CLAUDE.md` (đi lên từ thư mục hiện tại, rồi `~/.claude/CLAUDE.md`) có dòng bắt đầu bằng `Pre-PR review:`; khi có dòng đó, cả hai thắng `CODEX_THREAD_ID`. Không có dòng đó thì `project` rơi về `agent (không có dòng Pre-PR review)`, còn `agent+project` chạy đúng như `agent`: `codex` trong phiên Codex có `codex` trên PATH, nếu không thì `agent (không có dòng Pre-PR review)`. Với `agent+project`, `/znf:ship` review bằng Claude trước, Claude sạch thì gửi nhánh cho reviewer của project; `project` chỉ dùng reviewer của project. Kit chọn `codex` khi phiên đang chạy trong Codex (có `CODEX_THREAD_ID`) hoặc `ZNF_REVIEWER=codex`, và `codex` có trên PATH. Còn lại là `agent`; lý do là `ZNF_REVIEWER=agent` hoặc `codex không có trên PATH`.

## Ví dụ

```bash
zenify reviewer
zenify up --reviewer codex --apply
```

## Lưu ý

`ZNF_REVIEWER` nằm trong `env` của `<workspace>/.claude/settings.local.json`; `zenify up` seed giá trị `agent+project` (mặc định) khi chưa có và không đè giá trị bạn đã đặt. Workspace đang giữ `agent` (do kit cũ seed hay do bạn tự đặt bằng kit cũ) được chuyển sang `agent+project` một lần, ở lần đầu bản kit mới chạy trong workspace đó (đầu session hoặc `zenify up`). Giá trị đặt bằng `zenify reviewer set` hoặc `zenify up --reviewer` của bản kit mới không bao giờ bị chuyển, nên `zenify reviewer set agent` đưa về `agent` bất kỳ lúc nào. `zenify up --reviewer <agent|codex|project|agent+project>` ghi đè giá trị đó khi chạy kèm `--apply`; [`zenify reviewer set`](/reference/cli/zenify_reviewer_set) ghi đè ngay.
