---
summary: In thông tin để dùng env trên devbox k3s cục bộ: domain, file CA, URL, repo, image tag của từng app, tài khoản seed và cách vào cluster.
---
## Khi nào dùng

Khi cần mở một env hoặc đăng nhập vào nó: lấy URL từng app, biết image tag nào đang chạy, và tài khoản seed theo role. Sức khoẻ cluster, DNS và trust CA xem ở `zenify infra status --local`. Hướng dẫn dùng `kubectl`, ArgoCD, registry trên devbox: [Làm việc trên devbox k3s](/guides/local-k3s-dev).

## Kết quả

Lệnh in domain (lấy như `zenify infra status`) và đường dẫn bản sao CA `~/.zenify/infra/ca.crt` (dùng cho `NODE_EXTRA_CA_CERTS`). Với mỗi env `env-<slug>` (hoặc chỉ env của `--slug`): một dòng sync/health, rồi từng app có `svc` trong `infra.json` với URL `https://<svc>.<domain>` (env `base`) hoặc `https://<svc>-<slug>.<domain>` (env khác) theo domain của chính env đó, repo, và `image.tag` đọc từ values của Application con `<key>-<slug>-local` (`-` khi Application con chưa có). Chưa có env nào thì lệnh in `envs none`. Cuối cùng là tài khoản seed theo role, email và tenant domain lấy từ `seed` của `infra.json`, kèm mật khẩu `seed.password`; không có `seed.password` thì lệnh in `seed.passwordHint` (trường `loginHelp` trong JSON). Lệnh chỉ đọc (`kubectl get` với kubeconfig của k3s). `--slug` không tồn tại thì lệnh thoát khác 0 và liệt kê các env đang có. Mục `cluster` cho biết cách vào cluster: kubeconfig `/etc/rancher/k3s/k3s.yaml` cùng dòng `export KUBECONFIG=/etc/rancher/k3s/k3s.yaml` (file quyền `0640`, group chính của user devbox đọc được), context hiện tại, node và phiên bản k3s, registry nội bộ (alias `registry.zenify.internal:5000` cho pod, `10.43.200.10:5000` cho host) kèm ví dụ push/pull, ArgoCD (địa chỉ trong cluster, tài khoản `admin` với mật khẩu ban đầu đọc từ secret `argocd-initial-admin-secret`; secret đó đã xoá thì in lệnh đọc mật khẩu thay vào, `https://argocd.<domain>` chỉ khi Ingress đó tồn tại) và địa chỉ OpenBao chỉ khi có Service OpenBao. Context, node, ArgoCD, OpenBao chỉ có khi k3s (và ArgoCD) đã cài. Với `--json` lệnh in `{domain, ca, envs:[{slug,sync,health,apps:[{svc,url,repo,tag}]}], accounts:[{role,email,tenant,password}], loginHelp, cluster:{kubeconfig, export, kubeconfigNote, context, nodes:[{name,version}], registry:{alias,address,push,pull,list}, argocd:{url,password,passwordCommand,ingressUrl}, openbao}}`. Thiếu `infra.json` thì lệnh dừng, nêu đường dẫn file.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--local` | Bắt buộc: nhắm devbox k3s cục bộ. |
| `--slug` | Chỉ in env này. |
| `--json` | In JSON thay cho bảng chữ. |

## Ví dụ

```bash
zenify infra show --local
zenify infra show --local --slug base --json
```
