---
summary: In cấu hình worktree đã resolve của repo hiện tại, hoặc port sẽ cấp cho một key.
---
## Khi nào dùng

Khi bạn muốn xem repo đang khai base ref, dải port và cách seed deps nào, mà không mở `.claude/worktree.json`. Cũng dùng để biết trước port một slug sẽ nhận.

## Kết quả

Không có cờ, lệnh in từng trường một dòng: `abbrev`, `baseRef`, `worktreeDir`, `portEnv`, `portRange`, `deps`, `user`.

Với `--port <key>`, lệnh chỉ in số port sẽ cấp cho key đó trong dải của repo, bỏ qua các port đang có worktree giữ. Key thường là slug của task.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--port` | In port đã cấp cho key này thay cho toàn bộ cấu hình. |

## Ví dụ

```bash
zenify wt config
zenify wt config --port session-timeout
```

## Lưu ý

Ý nghĩa từng trường và cách chọn dải port cho repo mới: [Base ref và port block](/concepts/base-ref-and-ports).
