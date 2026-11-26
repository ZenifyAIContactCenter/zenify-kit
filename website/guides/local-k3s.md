---
title: Devbox k3s cục bộ
---

# Devbox k3s cục bộ với `zenify infra`

[`zenify infra`](/reference/cli/zenify_infra) dựng trên một máy Linux (Ubuntu Server, systemd, sudo không cần mật khẩu) một cluster k3s riêng, ArgoCD, chart `local-platform` và env `base` gồm các datastore, app và dữ liệu seed mà workspace khai báo. Mọi lệnh cần cờ `--local`.

## Trước khi chạy

- Knowledge store có `<store>/.config/infra.json`: repo helm, danh sách image cần build, các app của env và tài khoản seed. Binary của kit không chứa các giá trị này; schema xem ở [`zenify infra`](/reference/cli/zenify_infra). Thiếu file thì `up`, `status`, `show` dừng và nêu đường dẫn file.
- `gh auth login` xong: token của `gh` dùng để ArgoCD đọc repo helm và làm `NPM_TOKEN` khi build.
- Có `curl`, `git`, `gh`, `certutil` (gói `libnss3-tools`), `nft` (gói `nftables`), `resolvectl` (systemd-resolved), tối thiểu 6 GiB RAM và 30 GiB trống cho `/var/lib/rancher` khi cài lần đầu (chạy lại `up` trên devbox đã cài chỉ cần 10 GiB).

Thiếu điều kiện nào, [`zenify infra up --local`](/reference/cli/zenify_infra_up) in hết các vấn đề kèm cách sửa và dừng trước khi đổi gì trên máy.

Kubeconfig admin của k3s (`/etc/rancher/k3s/k3s.yaml`) có quyền `0640`, chỉ root và group chính của bạn đọc được; mọi lệnh `kubectl` của kit dùng file này, không dùng context mặc định của bạn.

## Dựng và kiểm tra

```bash
zenify infra config --local --domain devbox2.test   # tuỳ chọn; mặc định zenify.test
zenify infra up --local
zenify infra status --local
zenify infra show --local
```

[`zenify infra show --local`](/reference/cli/zenify_infra_show) in URL, repo, image tag đang chạy của từng app, tài khoản seed kèm tenant và mật khẩu (`seed.password` trong `infra.json`) và cách vào cluster (kubeconfig, registry, ArgoCD). Dùng `kubectl`, xem log, ArgoCD và registry trên devbox: xem [Làm việc trên devbox k3s](/guides/local-k3s-dev).

`up` in trước danh sách bước có đánh số, rồi hiện bước đang chạy và việc nó đang chờ; log đầy đủ nằm ở `~/.zenify/infra/logs/` (đường dẫn in ở đầu lệnh), `--verbose` in cả log build ra màn hình. `up` quét `<ws>/repos/*` lấy mọi repo có `Dockerfile` ở base ref và build thêm image cho chúng (sau các image trong `builds[]`). Một image build lỗi không dừng `up`: bước đó hiện `✗` kèm dòng lỗi cuối, các image sau vẫn build, rồi `up` in bảng `image · OK|FAIL|SKIP · thời gian · lý do`. App dùng image lỗi mà đã chạy thì giữ tag đang chạy (in `kept <app> at <tag>: image <repo> failed`), chưa chạy thì bị bỏ khỏi env (in `skipped <app>: image <repo> failed`), phần còn lại vẫn deploy và `state.json` vẫn ghi, nhưng `up` thoát khác 0 và nêu tên image lỗi cùng app bị bỏ; sửa nguyên nhân rồi chạy lại `up`, chỉ phần lỗi được build lại. Image seed lỗi thì `up` dừng trước khi deploy. `up` chạy lại được: bước đã xong in `unchanged`; bị Ctrl-C giữa chừng thì chạy lại `up` để tiếp tục. Khi đã có platform, `up` giữ domain và chart ref đang chạy; muốn đổi thì đặt rõ domain bằng `zenify infra config --local --domain <x>.test` hoặc chart ref bằng `zenify infra up --local --chart-ref <ref>`, nếu không lệnh dừng và báo khác biệt. `up --local --profile core|full` chọn bộ app của env: `infra.json` đánh dấu từng app bằng `profiles` (thiếu = cả hai, nên datastore và seed luôn có); app ngoài profile bị tắt (`enabled: false`), nên đổi từ `full` sang `core` gỡ các app chỉ thuộc `full`. Không có cờ thì `up` dùng profile đã lưu trong `state.json`, chưa có thì `core`; `status` và `show` chỉ liệt kê app của profile đó. Mỗi lần `up` làm mới config helm của env (hard refresh các Application, sync app seed để hook chạy, gắn commit đã sync vào pod annotation `zenify.dev/config-revision` nên pod roll khi config đổi). Khi xong, lệnh in URL các app (env `base` không có slug trong host, ví dụ `web.zenify.test`; env khác là `<svc>-<slug>.<domain>`), email, tenant domain và mật khẩu của tài khoản seed (`seed.password`; không có thì in `seed.passwordHint`). Trước pha 1, lệnh đặt CA nội bộ vào ConfigMap `local-ca` (key `ca.crt`) của namespace env để pod tin HTTPS của env. Nếu URL không mở được, [`zenify infra status --local`](/reference/cli/zenify_infra_status) cho biết DNS có resolve không và CA đã được tin chưa.

## Gỡ

[`zenify infra down --local`](/reference/cli/zenify_infra_down) chỉ in preview. Thêm `--apply` để xoá env và platform; thêm `--purge` để gỡ luôn k3s, CA, split DNS và firewall.
