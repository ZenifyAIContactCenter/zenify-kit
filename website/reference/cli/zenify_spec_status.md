---
title: zenify spec status
---

## zenify spec status

liệt kê trạng thái vòng đời từng spec (planned/in-progress/built/built?/superseded/unknown)

```
zenify spec status [flags]
```

### Options

```
      --active             ẩn spec đã superseded
      --base string        ref cơ sở dùng chung mọi repo (mặc định: baseRef mỗi repo)
  -h, --help               help for status
      --json               in JSON thay vì bảng
      --no-fetch           bỏ git fetch, dùng ref local
      --workspace string   thư mục workspace (mặc định cwd)
```

### SEE ALSO

* [zenify spec](./zenify_spec)	 - soi vòng đời spec (planned/built) và registry contract từ store spec

