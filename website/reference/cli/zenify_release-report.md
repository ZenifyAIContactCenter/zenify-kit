---
title: zenify release-report
---

## zenify release-report

sinh report rủi ro cho một release (chỉ-đọc, ghi docs/releases/R\<N\>.md)

```
zenify release-report [N] [flags]
```

### Options

```
  -h, --help               help for release-report
      --no-fetch           bỏ git fetch, dùng ref local
      --out-dir string     thư mục ghi report (mặc định repo docs/releases, tự tìm theo layout)
      --unreleased         ghi view release đang hình thành (release<latest>..staging) ra unreleased.md
      --verbose            hiện chore + commit chi tiết
      --workspace string   thư mục workspace (mặc định cwd)
```

### SEE ALSO

* [zenify](./zenify)	 - zenify — bộ công cụ workspace dùng chung của team

