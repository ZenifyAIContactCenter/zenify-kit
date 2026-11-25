---
summary: Dựng và quản lý devbox k3s cục bộ (k3s, ArgoCD, platform, env `base`) trên một máy Linux.
---
## Khi nào dùng

Khi bạn muốn chạy contact-center trên chính devbox Linux của mình: một cluster k3s riêng, ArgoCD, chart `local-platform` (registry, buildkitd, DNS, CA nội bộ) và env `base` với mongo, mysql, redis, `contact-center-be`, `contact-center-hub`, `contact-center-web`. Mọi lệnh con bắt buộc có `--local`; thiếu cờ này lệnh thoát mã 2 với `only --local is supported`.

## Kết quả

`zenify infra up --local` cài hoặc tiếp tục cài, `zenify infra status --local` đọc trạng thái, `zenify infra down --local` gỡ (mặc định chỉ preview), `zenify infra config --local` xem hoặc đặt domain và số env tối đa. Mọi lệnh `kubectl` đều chạy với `--kubeconfig /etc/rancher/k3s/k3s.yaml`, không bao giờ dùng context mặc định của bạn.

## Ví dụ

```bash
zenify infra up --local
zenify infra status --local --json
```
