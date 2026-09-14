---
title: zenify wt sweep
---

## zenify wt sweep

Dọn mọi worktree đã merge và sạch trong repo này (hoặc cả workspace với --all)

```
zenify wt sweep [flags]
```

### Options

```
      --all       sweep every wt-managed repo in the workspace (fetches each, 5s timeout, fail-open per repo)
  -n, --dry-run   report what would be removed without touching anything
  -f, --fetch     fetch origin first so merge state is current
  -h, --help      help for sweep
```

### SEE ALSO

* [zenify wt](./zenify_wt)	 - Quản lý git worktree + môi trường dev: tạo, liệt kê, gỡ, dọn worktree theo slug

