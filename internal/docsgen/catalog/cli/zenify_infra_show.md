---
summary: In thông tin để dùng env trên devbox k3s cục bộ: domain, file CA, URL, repo, image tag của từng app và tài khoản seed.
---
## Khi nào dùng

Khi cần mở một env hoặc đăng nhập vào nó: lấy URL từng app, biết image tag nào đang chạy, và tài khoản seed theo role. Sức khoẻ cluster, DNS và trust CA xem ở `zenify infra status --local`.

## Kết quả

Lệnh in domain (lấy như `zenify infra status`) và đường dẫn bản sao CA `~/.zenify/infra/ca.crt` (dùng cho `NODE_EXTRA_CA_CERTS`). Với mỗi env `env-<slug>` (hoặc chỉ env của `--slug`): một dòng sync/health, rồi từng app có `svc` trong `infra.json` với URL `https://<svc>.<domain>` (env `base`) hoặc `https://<svc>-<slug>.<domain>` (env khác) theo domain của chính env đó, repo, và `image.tag` đọc từ values của Application con `<key>-<slug>-local` (`-` khi Application con chưa có). Chưa có env nào thì lệnh in `envs none`. Cuối cùng là tài khoản seed theo role và tenant domain lấy từ `seed` của `infra.json`. Lệnh không bao giờ in mật khẩu, chỉ in `seed.passwordHint` (trường `loginHelp` trong JSON). Lệnh chỉ đọc (`kubectl get` với kubeconfig của k3s). `--slug` không tồn tại thì lệnh thoát khác 0 và liệt kê các env đang có. Với `--json` lệnh in `{domain, ca, envs:[{slug,sync,health,apps:[{svc,url,repo,tag}]}], accounts:[{role,email,tenant}], loginHelp}`. Thiếu `infra.json` thì lệnh dừng, nêu đường dẫn file.

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
