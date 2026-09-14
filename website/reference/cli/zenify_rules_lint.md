---
title: zenify rules lint
---

## zenify rules lint

chặn tiếng Việt trong file agent-read (skill .md, Go, rules)

### Synopsis

Không tham số thì quét asset skill của kit (internal/plugin/assets/znf); thêm --include-go để quét cả internal/**/*.go (đã dịch xong, gate bật). Truyền path cụ thể để quét nơi khác, vd: zenify rules lint ~/.zenify/knowledge/.config/rules

```
zenify rules lint [roots...] [flags]
```

### Options

```
  -h, --help         help for lint
      --include-go   quét cả internal/**/*.go
```

### SEE ALSO

* [zenify rules](./zenify_rules)	 - quản lý và kiểm rule team (F1/F2/F3)

