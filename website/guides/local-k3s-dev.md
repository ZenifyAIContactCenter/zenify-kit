---
title: Làm việc trên devbox k3s
---

# Làm việc trên devbox k3s

Trang này dành cho người đã dựng devbox bằng [Devbox k3s cục bộ](/guides/local-k3s) và cần tự xem, sửa, gỡ lỗi env bằng `kubectl`, ArgoCD và registry nội bộ.

## Bốn lệnh của `zenify infra`

- [`zenify infra up --local`](/reference/cli/zenify_infra_up): cài hoặc sửa k3s, ArgoCD, platform, CA, DNS, build image rồi dựng env `base`. Lệnh hiện từng bước đánh số với việc đang làm và thời gian chờ; log đầy đủ ở `~/.zenify/infra/logs/`. Chạy lại được; bước đã xong in `unchanged`.
- [`zenify infra status --local`](/reference/cli/zenify_infra_status): sức khoẻ k3s, ArgoCD, platform, từng env, DNS và trust CA.
- [`zenify infra show --local`](/reference/cli/zenify_infra_show): URL, repo, image tag của từng app, tài khoản seed, và mục `cluster` (kubeconfig, context, node, registry, ArgoCD, OpenBao nếu có).
- [`zenify infra down --local`](/reference/cli/zenify_infra_down): mặc định chỉ preview; `--apply` xoá env và platform, `--purge` gỡ luôn k3s, CA, DNS và firewall.

## kubectl với kubeconfig của devbox

k3s ghi kubeconfig admin ở `/etc/rancher/k3s/k3s.yaml`, quyền `0640`, đọc được bởi group chính của user devbox. Không cần `sudo`:

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl get nodes
```

Nếu máy còn kubeconfig của cluster khác (ví dụ cluster chung của team), luôn đặt `KUBECONFIG` như trên hoặc thêm `--kubeconfig /etc/rancher/k3s/k3s.yaml` vào từng lệnh, để không chạy nhầm lên cluster đó. Kit luôn dùng `--kubeconfig` này, không bao giờ dùng context mặc định.

## Pod, log và event của một env

Mỗi env nằm trong namespace `env-<slug>` (env mặc định là `env-base`):

```bash
kubectl -n env-base get pods
kubectl -n env-base logs deploy/<tên-deployment> --tail=200 -f
kubectl -n env-base describe pod <tên-pod>
kubectl -n env-base get events --sort-by=.lastTimestamp
kubectl -n env-base get jobs          # Job seed chạy ở pha 1
```

Platform (registry, buildkitd, `local-dns`, cert-manager) nằm trong namespace `zenify-system`.

## ArgoCD

ArgoCD quản lý mọi thứ qua Application: `local-platform`, root `env-<slug>` và các Application con `<key>-<slug>-local`.

```bash
kubectl -n argocd get applications
kubectl -n argocd get application env-base -o jsonpath='{.status.sync.status}/{.status.health.status}'
kubectl -n argocd annotate application env-base argocd.argoproj.io/refresh=hard --overwrite   # buộc so lại với repo helm
```

Mật khẩu admin ban đầu (user `admin`) có sẵn ở dòng `login:` của `zenify infra show --local`. Đã xoá secret ban đầu sau khi đổi mật khẩu thì `show` in lệnh dưới thay vào:

```bash
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d
```

Địa chỉ trong cluster là `https://argocd-server.argocd.svc`. Muốn mở UI từ trình duyệt trên devbox khi chưa có Ingress:

```bash
kubectl -n argocd port-forward svc/argocd-server 8080:443
# rồi mở https://localhost:8080
```

`zenify infra show` chỉ in `https://argocd.<domain>` khi Ingress đó tồn tại.

## Registry nội bộ

Registry chạy trong cluster tại ClusterIP `10.43.200.10:5000`. Pod kéo image qua alias `registry.zenify.internal:5000` (k3s chuyển alias này về ClusterIP), host truy cập thẳng ClusterIP qua HTTP:

```bash
curl http://10.43.200.10:5000/v2/_catalog
curl http://10.43.200.10:5000/v2/<name>/tags/list
```

Đẩy một image tự build (Docker cần khai `10.43.200.10:5000` trong `insecure-registries` vì registry dùng HTTP):

```bash
docker tag <image> 10.43.200.10:5000/<name>:<tag>
docker push 10.43.200.10:5000/<name>:<tag>
```

Trong manifest dùng `image: registry.zenify.internal:5000/<name>:<tag>`. `zenify infra up` tự build các image của `builds[]` qua buildkitd trong cluster, không cần Docker.

## DNS và CA từ devbox

- `up` cấu hình split DNS: mọi tên dưới `<domain>` (mặc định `zenify.test`) hỏi DNS của cluster trên interface `cni0`, qua unit `zenify-infra-dns.service`. Kiểm tra bằng `resolvectl query web.zenify.test`; kết quả đúng là IP Traefik `10.43.200.80`.
- CA nội bộ được tin trong kho hệ thống và NSS (Chrome, Firefox). Bản sao ở `~/.zenify/infra/ca.crt`, dùng cho Node: `export NODE_EXTRA_CA_CERTS=~/.zenify/infra/ca.crt`.
- Pod trong env đọc CA từ ConfigMap `local-ca` (key `ca.crt`) của namespace env.

## File cấu hình và trạng thái

- `infra.json` (repo helm, image cần build, app của env, tài khoản seed) nằm ở `<knowledge store>/.config/infra.json`, thường là `~/.zenify/knowledge/.config/infra.json`. Schema xem [`zenify infra`](/reference/cli/zenify_infra).
- `~/.zenify/infra/config.json`: domain, số env tối đa, chart ref (đổi bằng [`zenify infra config`](/reference/cli/zenify_infra_config)).
- `~/.zenify/infra/logs/up-<thời điểm>.log`, `down-<thời điểm>.log`: log đầy đủ của từng lần chạy, gồm log build (secret đã che).
- `~/.zenify/infra/state.json`: domain, phiên bản k3s, ArgoCD của lần `up` thành công gần nhất.

## Lỗi thường gặp

**k3s restart liên tục, một CRD kẹt `Established=False`.** Nếu k3s bị restart giữa lúc đang tạo CRD, CRD đó (thường là `addons.k3s.cattle.io`) kẹt ở `Installing` và k3s lỗi khoảng mỗi 60 giây. `up` không còn restart k3s khi nó chưa boot xong; khi gặp CRD kẹt, lệnh dừng và nêu tên CRD. Khôi phục bằng tay (kit không tự xoá CRD):

```bash
kubectl --kubeconfig /etc/rancher/k3s/k3s.yaml delete crd addons.k3s.cattle.io
sudo systemctl restart k3s
zenify infra up --local
```

**Build image hub lỗi 401 khi `npm install`.** Image hub cần token npm riêng (secret `npm_taskforcesh_token` trong `optionalSecrets`). Thiếu token thì secret không được truyền và registry npm trả 401. Đặt `NPM_TASKFORCESH_TOKEN` trong môi trường hoặc lưu bằng [`zenify env set`](/reference/cli/zenify_env_set), rồi chạy lại `up`.

**`up` dừng vì lệch domain hoặc chart ref.** Platform đang chạy với giá trị khác. Đặt rõ bằng `zenify infra config --local --domain <x>.test` hoặc `zenify infra up --local --chart-ref <ref>`.

**URL không mở được.** Chạy `zenify infra status --local`: dòng DNS cho biết tên có resolve về Traefik không, dòng CA cho biết CA đã được tin chưa.

**Application `Degraded` hoặc `Progressing` mãi.** Xem `kubectl -n env-<slug> get events --sort-by=.lastTimestamp` và log pod lỗi; image chưa có trong registry thì kiểm tra bằng `curl http://10.43.200.10:5000/v2/<name>/tags/list`.

## OpenBao (Lát 2, sắp có)

OpenBao để giữ secret của env sẽ có ở lát sau. Khi Service OpenBao đã chạy trong cluster, `zenify infra show` in thêm dòng `openbao` với địa chỉ trong cluster; trước đó dòng này không xuất hiện.
