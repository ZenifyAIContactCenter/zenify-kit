---
title: zenify down
---

## zenify down

Offboard: gỡ znf global hooks, .worktrees/ + .wt/ excludes, và owned settings skeletons (preview mặc định; --apply để thực thi)

```
zenify down [flags]
```

### Options

```
      --apply              thực thi gỡ (mặc định chỉ preview)
  -h, --help               help for down
      --manifest string    path to repos.yaml (default: manifest/repos.yaml under cwd when present, else the copy embedded in the binary)
      --overlay string     path to personal overlay
      --workspace string   workspace root directory (default ".")
```

### SEE ALSO

* [zenify](./zenify)	 - zenify — bộ công cụ workspace dùng chung của team

