---
summary: In trạng thái devbox k3s cục bộ: k3s, ArgoCD, platform, các env, DNS và CA.
---
## Khi nào dùng

Sau `zenify infra up --local`, hoặc khi một URL env không mở được, để biết phần nào chưa Synced/Healthy.

## Kết quả

Lệnh in phiên bản k3s và ArgoCD (hoặc `not installed`), sync/health của `local-platform` và của từng Application `env-<slug>` kèm URL web, be, hub. `dns.resolves` lấy từ một lần `resolvectl query web-<slug>.<domain>` thật và chỉ đúng khi câu trả lời là IP ingress của cluster (`10.43.200.80`), không tính wildcard `*.test` của resolver khác; `ca.trusted` đúng khi CA có trong kho CA hệ thống và trong NSS DB của bạn. Với `--json` lệnh in object `{k3s, argocd, platform:{sync,health}, envs:[{slug,sync,health,urls}], dns:{host,resolves}, ca:{trusted}}`.

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
