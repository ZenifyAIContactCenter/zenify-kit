---
summary: Gỡ các env và platform khỏi devbox k3s cục bộ; `--purge` gỡ luôn k3s, CA và DNS.
---
## Khi nào dùng

Khi muốn dọn devbox để dựng lại, hoặc trả máy về như trước khi cài.

## Kết quả

Mặc định lệnh chỉ in preview có chữ `PREVIEW` và không chạy lệnh nào. Với `--apply`, lệnh xoá từng Application `env-<slug>` (chờ finalizer xoá hết tài nguyên) và namespace của nó, rồi xoá `local-platform` cùng tài nguyên và namespace `zenify-system`. Không có k3s hoặc ArgoCD thì lệnh in `nothing to remove` cho phần cluster. Lệnh hiện từng bước như `zenify infra up` (`detect`, `envs`, `platform`, và với `--purge` thêm `trust-dns`, `k3s`, `state`), spinner trên terminal, một dòng mỗi lần đổi trạng thái khi không phải terminal, và ghi log vào `~/.zenify/infra/logs/down-<thời điểm>.log`. Thêm `--purge`, lệnh gỡ CA khỏi kho hệ thống và NSS, gỡ unit split DNS, chạy `k3s-uninstall.sh`, gỡ firewall của k3s và xoá `~/.zenify/infra/state.json`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--local` | Bắt buộc: nhắm devbox k3s cục bộ. |
| `--apply` | Thực thi. Không có cờ này lệnh chỉ in preview. |
| `--purge` | Gỡ thêm CA, split DNS, k3s, firewall và file state. |

## Ví dụ

```bash
zenify infra down --local
zenify infra down --local --apply
zenify infra down --local --purge --apply
```
