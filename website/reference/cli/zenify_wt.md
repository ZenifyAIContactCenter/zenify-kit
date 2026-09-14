---
title: zenify wt
---

## zenify wt

Quản lý git worktree + môi trường dev: tạo, liệt kê, gỡ, dọn worktree theo slug

### Options

```
  -h, --help   help for wt
```

### SEE ALSO

* [zenify](./zenify)	 - zenify — bộ công cụ workspace dùng chung của team
* [zenify wt config](./zenify_wt_config)	 - Hiện worktree.json đã resolve (hoặc --port \<key\> để xem port đã cấp)
* [zenify wt ls](./zenify_wt_ls)	 - Liệt kê worktree trong repo này (git ⋈ state), kèm trạng thái running/merged
* [zenify wt new](./zenify_wt_new)	 - Tạo worktree: branch + port + env đã seed + deps
* [zenify wt path](./zenify_wt_path)	 - In đường dẫn tuyệt đối của worktree theo slug
* [zenify wt promote](./zenify_wt_promote)	 - Chuyển node_modules symlink của worktree thành bản copy CoW riêng
* [zenify wt rm](./zenify_wt_rm)	 - Gỡ một worktree (từ chối worktree dirty/detached/chưa merge nếu không có --force)
* [zenify wt sweep](./zenify_wt_sweep)	 - Dọn mọi worktree đã merge và sạch trong repo này (hoặc cả workspace với --all)
* [zenify wt url](./zenify_wt_url)	 - In http://localhost:\<port\> của một slug
* [zenify wt wire](./zenify_wt_wire)	 - Trỏ file env của worktree này sang các peer service đang được sửa

