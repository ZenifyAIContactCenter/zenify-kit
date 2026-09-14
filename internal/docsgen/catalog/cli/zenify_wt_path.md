---
summary: In đường dẫn tuyệt đối của worktree theo slug.
---
## Khi nào dùng

Khi bạn hoặc một script cần `cd` vào worktree của một task mà không nhớ đường dẫn.

## Kết quả

Lệnh in đúng một dòng là đường dẫn, không in gì khác, nên có thể dùng trực tiếp trong shell. Slug không tồn tại trả lỗi `wt: no worktree "<slug>"`.

## Ví dụ

```bash
cd "$(zenify wt path session-timeout)"
```
