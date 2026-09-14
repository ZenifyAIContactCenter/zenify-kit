---
title: zenify observe statusline
---

## zenify observe statusline

HUD statusline: hiện model · ctx% · ⟳dispatch · ↓tool-output · $cost

### Synopsis

Vẽ một dòng statusline (HUD) cho Claude Code từ JSON stdin cộng state
observe của kit theo phiên (dispatch count từ `zenify observe count` và
tool-output volume từ `zenify observe meter`).

Đấu nối trong settings.json (statusLine là một key trong settings.json — plugin
không tự khai được key này, và chỉ được phép MỘT statusline, nên cái này sẽ thay
statusline hiện có):

  "statusLine": { "type": "command", "command": "zenify observe statusline" }

Segment (ẩn khi trống): model · ctx% · ⟳dispatches · ↓tool-output/calls · $cost.

--segment CHỈ render hai segment riêng của kit (⟳dispatches · ↓tool-output)
và bỏ model/ctx/cost. Dùng khi bạn đã có sẵn statusline ưng ý: giữ nguyên
script của bạn, pipe cùng JSON stdin vào đây, rồi nối output vào — ví dụ:

  seg=$(printf '%s' "$input" | zenify observe statusline --segment)
  [ -n "$seg" ] && line2+="  |  $seg"

```
zenify observe statusline [flags]
```

### Options

```
  -h, --help      help for statusline
      --segment   render only ⟳dispatch · ↓tool-output, for splicing into an existing statusline
```

### SEE ALSO

* [zenify observe](./zenify_observe)	 - Observability: đếm/nhắc fan-out subagent
* [zenify observe statusline install](./zenify_observe_statusline_install)	 - Ghi key statusLine → `zenify observe statusline` vào ~/.claude/settings.json (chỉ khi trống)

