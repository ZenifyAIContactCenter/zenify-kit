---
summary: Gỡ phần ZenifyKit đã cài vào máy và repo, giữ nguyên code, workspace và knowledge store.
---
## Khi nào dùng

Khi bạn rời workspace, đổi máy, hoặc cần cài lại từ đầu. Đây là bước đảo của `zenify up`.

## Kết quả

Lệnh gỡ bốn thứ: hook `znf` và biến `env.CLAUDE_CODE_SUBAGENT_MODEL` (chỉ khi còn đúng giá trị kit ghi) trong `~/.claude/settings.json`, các dòng exclude `.worktrees/` và `.wt/` trong từng repo, và file `.claude/settings.local.json` do kit sinh ra trong từng repo. Mặc định lệnh chỉ in preview có tiêu đề `PREVIEW`. Với `--apply` lệnh thực thi.

Lệnh không đụng vào repo đã clone, thư mục `.zenify/` hay knowledge store.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--apply` | Thực thi thay đổi. Không có cờ này lệnh chỉ in preview. |
| `--manifest` | Đường dẫn manifest workspace, khi không dùng bản mặc định. |
| `--overlay` | File overlay cho máy này. |
| `--workspace` | Thư mục workspace, mặc định là thư mục hiện tại. |

## Ví dụ

```bash
zenify down
zenify down --apply
```
