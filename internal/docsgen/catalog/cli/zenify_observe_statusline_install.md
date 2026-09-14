---
summary: Ghi key `statusLine` trỏ tới `zenify observe statusline` vào `~/.claude/settings.json`.
---
## Khi nào dùng

Khi bạn chưa có statusline riêng và muốn dùng statusline của kit.

## Kết quả

Lệnh chỉ ghi khi key `statusLine` còn trống. Đã cấu hình đúng thì báo "đã cấu hình sẵn". Đã có statusline khác thì từ chối và gợi ý hai cách: ghép đoạn bằng `zenify observe statusline --segment`, hoặc đè bằng `--force`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--force` | Đè statusline hiện có. |

## Ví dụ

```bash
zenify observe statusline install
```
