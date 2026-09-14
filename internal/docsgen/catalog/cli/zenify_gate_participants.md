---
summary: Liệt kê các repo tham gia contract gate cùng cách mỗi repo truy cập DB chung.
---
## Khi nào dùng

Khi bạn muốn biết một thay đổi trên collection, endpoint hoặc queue chung sẽ được quét ở những repo nào. Skill `/znf:gate` đọc danh sách này.

## Kết quả

Mỗi dòng là một repo với kiểu truy cập DB và các pattern truy cập. Danh sách gộp từ hai nguồn: file `gate-participants.json` trong knowledge store và các repo khai `gate.sharedStore=true` trong `.claude/worktree.json`. Repo có trong file nhưng không có checkout dưới workspace được báo riêng.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--workspace` | Thư mục workspace. Mặc định thư mục hiện tại. |
| `--json` | In JSON. |

## Ví dụ

```bash
zenify gate participants --workspace ~/WorkingSpace/zenify
```
