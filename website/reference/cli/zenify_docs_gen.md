---
title: zenify docs gen
---

## zenify docs gen

sinh reference cho site docs (CLI từ cobra, skill/agent từ frontmatter, bảng hook)

### Synopsis

Ghi markdown VitePress vào --out (mặc định website/reference). Với --check không ghi, chỉ so với file trên đĩa; lệch → exit 1 và in danh sách. CI chạy --check.

```
zenify docs gen [flags]
```

### Options

```
      --check        chỉ so, không ghi; lệch → exit 1
  -h, --help         help for gen
      --out string   thư mục đích (default "website/reference")
```

### SEE ALSO

* [zenify docs](./zenify_docs)	 - quản lý docs layer (agent-managed, dev read-only)

