---
summary: In trạng thái devbox k3s cục bộ: k3s, ArgoCD, platform, các env, DNS và CA.
---
## Khi nào dùng

Sau `zenify infra up --local`, hoặc khi một URL env không mở được, để biết phần nào chưa Synced/Healthy.

## Kết quả

Lệnh in phiên bản k3s và ArgoCD (hoặc `not installed`), sync/health của `local-platform` và của từng Application `env-<slug>` kèm URL của các app có `svc` trong `infra.json`, theo domain của chính env đó. Domain mặc định lấy như `zenify infra up`: config, `state.json`, platform đang chạy, rồi `zenify.test`. `dns.resolves` lấy từ một lần `resolvectl query` thật cho host của app đầu tiên trong env đầu tiên, trên domain của chính env đó (`<svc>.<domain>` với env `base`, `<svc>-<slug>.<domain>` với env khác; chưa có env thì dùng domain mặc định) và chỉ đúng khi câu trả lời là IP ingress của cluster (`10.43.200.80`), không tính wildcard `*.test` của resolver khác; `ca.trusted` chỉ đúng khi CA trong `~/.zenify/infra/ca.crt` trùng với file CA hệ thống, nằm trong bundle `/etc/ssl/certs/ca-certificates.crt`, và trùng với cert trong NSS DB của bạn có cờ tin cậy `C`; kho nào giữ CA khác hoặc thiếu đều cho `false`. Với `--json` lệnh in object `{k3s, argocd, platform:{sync,health}, envs:[{slug,sync,health,urls}], dns:{host,resolves}, ca:{trusted}}`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--local` | Bắt buộc: nhắm devbox k3s cục bộ. |
| `--json` | In JSON thay cho bảng chữ. |

## Ví dụ

```bash
zenify infra status --local
zenify infra status --local --json
```
