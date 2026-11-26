---
title: zenify infra
---

[Lệnh CLI](/reference/cli/) / `zenify infra`

# `zenify infra`

Dựng và quản lý devbox k3s cục bộ (k3s, ArgoCD, platform, env `base`) trên một máy Linux, theo cấu hình `infra.json` của workspace.

## Cú pháp

```text
zenify infra
```

## Khi nào dùng

Khi bạn muốn chạy stack của workspace trên chính devbox Linux của mình: một cluster k3s riêng, ArgoCD, chart `local-platform` (registry, buildkitd, DNS, CA nội bộ) và env `base` gồm các app mà `infra.json` khai báo. Mọi lệnh con bắt buộc có `--local`; thiếu cờ này lệnh thoát mã 2 với `only --local is supported`.

## Kết quả

`zenify infra up --local` cài hoặc tiếp tục cài, `zenify infra status --local` đọc trạng thái, `zenify infra show --local` in URL, image tag và tài khoản seed của từng env, `zenify infra down --local` gỡ (mặc định chỉ preview), `zenify infra config --local` xem hoặc đặt domain và số env tối đa. Mọi lệnh `kubectl` đều chạy với `--kubeconfig /etc/rancher/k3s/k3s.yaml`, không bao giờ dùng context mặc định của bạn. Hai lần `up`/`down --apply` không chạy cùng lúc: lần sau dừng với lỗi nêu file khoá `~/.zenify/infra/infra.lock`.

## File `infra.json`

Binary của kit công khai nên không chứa repo, image hay tài khoản nào của workspace. `up`, `status` và `show` đọc chúng từ `<knowledge store>/.config/infra.json` (cùng chỗ với `gate-participants.json`); thiếu file hoặc thiếu trường thì lệnh dừng, nêu đường dẫn file và các trường còn thiếu, không có giá trị mặc định nào trong binary.

| Trường | Ý nghĩa |
|---|---|
| `helmRepoURL` | Repo git chứa `charts/local-platform`, `charts/app-of-apps` và `envs/`. |
| `builds[]` | Image build theo thứ tự (image gốc trước image `FROM` nó): `repo`, `context`, `dockerfile`, `target` (tuỳ chọn), `image` (đường dẫn dưới registry nội bộ), `tag` (cố định và được dùng lại nguyên trạng, nên đổi `args`/`context` thì phải đổi `tag`; bỏ trống thì `sha-<commit>-v<hash>` với hash gồm `context`, `dockerfile`, `target` và mọi build arg, nên sửa bất kỳ giá trị nào trong số đó đều build image mới), `args`, `envArgs` (build arg đổi theo env, giá trị là một biến `slug`, `domain`, `registry` hoặc `baseOnly` = `1` cho env `base`, `0` cho env khác; không dùng cùng `tag` cố định), `secrets` (bắt buộc), `optionalSecrets` (chỉ truyền khi có). |
| `envApps[]` | Key app-of-apps của env theo thứ tự: `key` (không bắt đầu bằng `env-`), `phase` (`1` datastore và seed, `2` app), `repo` (repo build ra `image.tag`: có trong `builds[]` hoặc là repo quét được có `Dockerfile` ở base ref; `up` dừng ở bước `buildctl` trước khi build nếu app thuộc profile đang chọn mà repo không thuộc hai nhóm đó, app ngoài profile không bị kiểm), `svc` (tiền tố host), `ingress[]` (`values` là key values dạng chấm, `path` là đường dẫn URL), `seed: true` cho đúng một app nhận `slug`, `domain`, `seed.tag`, `profiles[]` (tuỳ chọn, mỗi giá trị `core` hoặc `full`; thiếu = cả hai, nên datastore và seed không cần khai). `up --profile` chỉ dựng app thuộc profile đang chọn; app ngoài profile vẫn có trong values của app-of-apps nhưng `enabled: false` (không cần tag), nên đổi profile từ `full` sang `core` tắt app chỉ thuộc `full`; không chờ, không tính vào `skipped`/`kept`; `status` và `show` cũng chỉ liệt kê app của profile đang lưu. |
| `scan` | Tuỳ chọn. Cấu hình build mặc định cho repo trong `<workspace>/repos` có `Dockerfile` ở gốc tại base ref nhưng không có trong `builds[]`: `exclude[]` (repo không bao giờ được quét), `args` (build arg của mọi image quét được), `optionalSecrets[]` (chỉ truyền khi có). Image quét được dùng context `.`, `Dockerfile`, image `<repo>`; thứ tự là `builds[]` trước, rồi các repo quét được theo tên. |
| `seed` | `repo` (build image seed), `tenant`, `password` (mật khẩu test cố định của dữ liệu seed, `show` và `up` in ra), `passwordHint` (chỗ ghi mật khẩu, bắt buộc khi không có `password`), `accounts[]` (`role`, `email`). |

File nằm trong store dùng chung của team nên kit kiểm mọi giá trị trước khi dùng, và liệt kê mọi trường sai cùng lúc, mỗi trường kèm luật:

- `helmRepoURL`: URL `https://github.com/<owner>/<repo>` không chứa thông tin đăng nhập (token `gh` của bạn được gửi tới repo này).
- `builds[].repo`: khớp `^[a-z0-9][a-z0-9._-]*$`, không có `..`, không trùng; `context` và `dockerfile`: đường dẫn tương đối nằm trong repo; `image`: phần chữ thường `[a-z0-9]` nối bằng `.`, `_`, `-`, `/`; `tag`: khớp `^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`; `secrets[]`, `optionalSecrets[]`: khớp `^[a-z0-9_]+$`.
- `scan.exclude[]`: khớp `^[a-z0-9][a-z0-9._-]*$`, không có `..`; `scan.optionalSecrets[]`: khớp `^[a-z0-9_]+$`; `scan.args`: key không rỗng. Thư mục trong `repos/` có tên không hợp lệ bị bỏ qua kèm cảnh báo.
- `builds[].image` không trùng giữa hai build; `tag` cố định không đi cùng `envArgs`.
- `envApps[]`: `key` không được là `env`, `environment`, `global`, `apps` (là values của app-of-apps); app `seed` ở `phase: 1` và không có `repo`, `svc`, `ingress`; app có `repo` ở `phase: 2` (chạy sau seed); `ingress[].values` không trỏ vào `image` (key `image.tag` của kit); mỗi giá trị của `profiles[]` là `core` hoặc `full`.
- `envApps[].key` và `svc`: nhãn DNS (`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`) dài tối đa 30 byte, để `<svc>-<slug>` với slug dài nhất (32) vẫn ≤ 63; `key` không trùng và không bắt đầu bằng `env-`; `ingress[].path` bắt đầu bằng `/`.

## Ví dụ

```bash
zenify infra up --local
zenify infra status --local --json
```

## Lệnh liên quan

- [zenify](./zenify): Binary dòng lệnh của ZenifyKit, chứa mọi lệnh workspace và plugin skill đi kèm.
- [zenify infra config](./zenify_infra_config): Xem hoặc đặt domain và số env tối đa của devbox k3s cục bộ.
- [zenify infra down](./zenify_infra_down): Gỡ các env và platform khỏi devbox k3s cục bộ; `--purge` gỡ luôn k3s, CA và DNS.
- [zenify infra show](./zenify_infra_show): In thông tin để dùng env trên devbox k3s cục bộ: domain, file CA, URL, repo, image tag của từng app, tài khoản seed và cách vào cluster.
- [zenify infra status](./zenify_infra_status): In trạng thái devbox k3s cục bộ: k3s, ArgoCD, platform, các env, DNS và CA.
- [zenify infra up](./zenify_infra_up): Cài hoặc tiếp tục cài devbox k3s cục bộ tới env `base` chạy được, chạy lại an toàn.

<!-- Generated by zenify docs gen: prose from internal/docsgen/catalog, facts from the command tree. -->
