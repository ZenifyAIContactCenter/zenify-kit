---
summary: In các file render UI đã đổi so với base, mỗi dòng một file; file test không tính.
---
## Khi nào dùng

Bước 4 của `/znf:ship` chạy với `--count` để quyết định có cần `znf:ui-verifier`. Cùng tập file mà `zenify ui-verify check` chặn, nên hai nơi không lệch nhau.

## Ví dụ

```bash
zenify ui-verify files --base "$(zenify wt base)" --count
```
