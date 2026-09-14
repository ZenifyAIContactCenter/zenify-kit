---
summary: Vẽ một dòng statusline cho Claude Code với model, ngữ cảnh, số dispatch, tool output và chi phí.
---
## Khi nào dùng

Claude Code gọi lệnh này qua key `statusLine` trong `~/.claude/settings.json`. Bạn cài bằng lệnh con `install` hoặc tự ghép vào statusline sẵn có.

## Kết quả

Một dòng gồm các đoạn, ẩn khi trống: model, phần trăm ngữ cảnh, số dispatch subagent, lượng tool output và chi phí. Dữ liệu vào là JSON mà Claude Code đưa qua stdin cộng với state observe của phiên.

Claude Code chỉ cho phép một statusline. Nếu bạn đã có script riêng, giữ script đó và nối thêm đoạn của kit:

```bash
seg=$(printf '%s' "$input" | zenify observe statusline --segment)
[ -n "$seg" ] && line2+="  |  $seg"
```

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--segment` | Chỉ in hai đoạn của kit, số dispatch và tool output, để ghép vào statusline sẵn có. |
