---
summary: Thêm một việc thủ công vào backlog và in `track: <bid>`.
---
## Khi nào dùng

Khi có việc cần làm sau; bid dạng `b-<YYMMDD>-<4 hex>`. `start --from <bid>` đưa việc ra khỏi backlog.

## Ví dụ

```bash
zenify track backlog add "gộp hai helper" --repo kit --est M --ref memory:foo
```
