---
summary: Dọn mọi worktree đã merge và sạch trong repo, dừng dev server của chúng trước khi gỡ.
---
## Khi nào dùng

Sau khi PR đã merge, hoặc định kỳ khi workspace tích nhiều worktree cũ. Hook đầu session in một dòng nhắc số worktree có thể dọn.

## Kết quả

Với mỗi worktree, lệnh quyết định gỡ hay giữ và in lý do. Chỉ gỡ worktree do `wt` tạo, đang ở branch thật, branch đã có dấu vết merge trong base và cây làm việc sạch. Trước khi gỡ, nó dừng dev server đang giữ port của worktree đó. Mục state trỏ tới thư mục đã mất cũng được dọn. Cuối cùng in `wt: swept N, left M`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `-n`, `--dry-run` | Chỉ báo sẽ gỡ gì, không đụng vào đâu. |
| `-f`, `--fetch` | Fetch origin trước để trạng thái merge không bị cũ. |
| `--all` | Dọn mọi repo có cấu hình worktree trong workspace. Luôn fetch từng repo, mỗi repo tối đa 5 giây, repo lỗi bị bỏ qua. |

## Ví dụ

```bash
zenify wt sweep --dry-run
zenify wt sweep --fetch
zenify wt sweep --all
```

## Lưu ý

Không fetch mà base cũ, một branch đã merge có thể bị đọc là chưa merge và được giữ lại. Khi thấy nhiều mục "left alone" bất ngờ, chạy lại với `--fetch`.
