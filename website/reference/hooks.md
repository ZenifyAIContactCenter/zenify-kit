---
title: Hook
---

# Hook

`zenify up` ghi các hook sau vào `~/.claude/settings.json`; mỗi hook là một lệnh `zenify hooks-run <id>` (fail-open, chỉ chạy trong workspace).

| Event | Matcher | Lệnh |
|---|---|---|
| SessionStart | — | `zenify hooks-run session-start` |
| SessionStart | — | `zenify hooks-run git-state` |
| SessionStart | — | `zenify hooks-run wt-report` |
| Stop | — | `zenify hooks-run docs-sync` |
| Stop | — | `zenify hooks-run git-state-stop` |
| PreToolUse | `Task\|Agent` | `zenify hooks-run observe-count` |
| PostToolUse | `Task\|Agent\|Bash\|WebFetch\|WebSearch\|Read` | `zenify hooks-run observe-meter` |
| PreToolUse | `Bash` | `zenify git-guard` (cài bằng `zenify guard install`) |

## Nguồn

Sinh bởi `zenify docs gen` từ bảng hook trong binary (`internal/apply/globalhooks.go`).
