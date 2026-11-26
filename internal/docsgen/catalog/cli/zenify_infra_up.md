---
summary: Cài hoặc tiếp tục cài devbox k3s cục bộ tới env `base` chạy được, chạy lại an toàn.
---
## Khi nào dùng

Lần đầu dựng devbox, hoặc chạy lại sau khi một bước lỗi giữa chừng. Bước đã xong được nhận ra và in `unchanged`, không cài lại k3s hay ArgoCD.

## Kết quả

Lệnh chạy preflight trước: mọi vấn đề (OS, systemd, sudo, RAM, đĩa, phiên bản k3s, `build-secrets` khi không có `NPM_TOKEN` và `gh auth token` cũng lỗi, tool thiếu) được in cùng cách sửa, rồi lệnh thoát khác 0 trước khi đổi bất cứ thứ gì. Lệnh đọc `infra.json` trước cả preflight (thiếu file thì dừng). Qua preflight, lệnh giữ khoá `~/.zenify/infra/infra.lock`, rồi lần lượt: cài hoặc khôi phục k3s (ghi config, registry mirror, firewall trước khi chạy installer để lần khởi động đầu đọc đúng; chỉ restart k3s khi đã boot xong: API trả lời, node Ready, các CRD `addons.k3s.cattle.io`, `helmcharts.helm.cattle.io`, `helmchartconfigs.helm.cattle.io` Established; CRD kẹt `Established=False` thì dừng và in cách khôi phục, kể cả thử lại lần restart lỗi trước đó), xác định domain và chart ref rồi kiểm lệch so với platform đang chạy, cài ArgoCD (cài lại manifest pin khi một workload ArgoCD còn thiếu hoặc lệch phiên bản), lưu token `gh auth token` cho repo helm (không in token), apply `local-platform` và chờ Synced/Healthy, tin CA nội bộ (hệ thống + NSS), cấu hình split DNS cho `<domain>` trên `cni0` và luôn restart unit `zenify-infra-dns`, build các image trong `builds[]` theo thứ tự vào registry nội bộ (bỏ qua tag đã có), tạo hoặc cập nhật ConfigMap `local-ca` (key `ca.crt`, CA nội bộ) trong namespace `env-base`, apply env `base` pha 1 (chỉ các app `phase: 1`, tức datastore và seed, để seed chạy trên DB rỗng), chờ `env-base` rồi từng Application con của pha 1 (tới khi Job seed xong), rồi pha 2 (bật các app `phase: 2`) và chờ `env-base` cùng mọi Application con. Env đã có bất kỳ app pha 2 nào thì bỏ pha 1. Đầu lệnh in danh sách bước có đánh số (`preflight`, `k3s`, `target`, `argocd`, `repo-credential`, `platform`, `ca`, `dns`, `buildctl`, `build <image>` cho từng image, `env-ca`, `env-phase-1`, `env-phase-2`, `state`), mỗi bước `pending`, cùng đường dẫn log `~/.zenify/infra/logs/up-<thời điểm>.log`. Trên terminal, bước đang chạy có spinner kèm việc đang làm (ví dụ `waiting application env-base Synced/Healthy`), thời gian đã chờ và dòng cuối của log build; khi xong in `✓` kèm `changed`/`unchanged` và thời gian, `•` cho bước bỏ qua (tag đã có, pha 1 khi app đã chạy), `✗` kèm lỗi. Khi không phải terminal (pipe, CI, `NO_COLOR`), lệnh không in spinner hay mã ANSI mà in một dòng cho mỗi lần đổi trạng thái hoặc việc đang làm. Log build chỉ hiện dạng một dòng cuối trên spinner, đầy đủ trong file log (secret đã che); `--verbose` in hết ra màn hình. Lỗi nêu tên bước, in đường dẫn log và thoát khác 0. Ctrl-C dừng spinner, in bước đang dừng và nhắc chạy lại `zenify infra up --local` để tiếp tục. Chỉ khi xong hết lệnh mới ghi `~/.zenify/infra/state.json` và in URL các app cùng email, tenant domain và mật khẩu `seed.password` của tài khoản seed (không có thì in `seed.passwordHint`).

Domain lấy theo thứ tự: `zenify infra config`, `state.json`, platform đang chạy (`local-platform`), rồi `zenify.test`. Env `base` dùng host không có slug (`<svc>.<domain>`; build arg có biến `baseOnly` nhận `1`), env khác dùng `<svc>-<slug>.<domain>`. Chart ref lấy theo thứ tự: `--chart-ref`, config, platform đang chạy, rồi `main`. Nếu giá trị không đến từ cờ hay config mà khác platform đang chạy, lệnh dừng trước khi đổi gì, in cả hai giá trị và cách đổi có chủ đích. Khi chạy lại, lệnh giữ `createdAt` của lần cài đầu. `--ref` với repo không có trong `builds[]` thoát mã 2 và liệt kê repo hợp lệ.

Lệnh chỉ nhận trạng thái Synced/Healthy khi Argo đã so sánh với đúng spec vừa apply (`status.sync.comparedTo.source` trùng `spec.source`), và chờ Application con chưa được tạo tới hết thời gian chờ.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--local` | Bắt buộc: nhắm devbox k3s cục bộ. |
| `--verbose` | In mọi dòng log build ra màn hình (file log luôn có đủ). |
| `--chart-ref` | Ref của repo helm cho chart platform và env. Mặc định là `chartRef` trong config, không có thì ref của platform đang chạy, không có nữa thì `main`. |
| `--ref` | `<repo>=<ref>` để build repo đó từ ref khác `baseRef` của nó; lặp lại được. Không có `--ref` thì dùng `baseRef` trong `.claude/worktree.json` của repo; repo không có file đó (máy mới) thì build từ nhánh mặc định của origin (`origin/HEAD`, tự chạy `git remote set-head origin --auto` một lần nếu chưa đặt). File có nhưng lỗi thì lệnh dừng. |

## Ví dụ

```bash
zenify infra up --local
zenify infra up --local --chart-ref feat/local-env --ref my-web=origin/feat/x
```
