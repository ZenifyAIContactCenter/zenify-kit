---
title: Devbox k3s cục bộ
---

# Devbox k3s cục bộ với `zenify infra`

[`zenify infra`](/reference/cli/zenify_infra) dựng trên một máy Linux (Ubuntu Server, systemd, sudo không cần mật khẩu) một cluster k3s riêng, ArgoCD, chart `local-platform` và env `base` gồm mongo, mysql, redis, `contact-center-be`, `contact-center-hub`, `contact-center-web` với dữ liệu seed. Mọi lệnh cần cờ `--local`.

## Trước khi chạy

- `gh auth login` xong: token của `gh` dùng để ArgoCD đọc repo helm và làm `NPM_TOKEN` khi build.
- Có `curl`, `git`, `gh`, `certutil` (gói `libnss3-tools`), tối thiểu 6 GiB RAM và 30 GiB trống cho `/var/lib/rancher`.

Thiếu điều kiện nào, [`zenify infra up --local`](/reference/cli/zenify_infra_up) in hết các vấn đề kèm cách sửa và dừng trước khi đổi gì trên máy.

## Dựng và kiểm tra

```bash
zenify infra config --local --domain quyentm.test   # tuỳ chọn; mặc định zenify.test
zenify infra up --local
zenify infra status --local
zenify infra show --local
```

[`zenify infra show --local`](/reference/cli/zenify_infra_show) in URL, repo, image tag đang chạy của từng app và tài khoản seed (không in mật khẩu).

`up` chạy lại được: bước đã xong in `unchanged`. Khi đã có platform, `up` giữ domain và chart ref đang chạy; muốn đổi thì đặt rõ bằng `zenify infra config --local --domain <x>.test` hoặc `--chart-ref <ref>`, nếu không lệnh dừng và báo khác biệt. Khi xong, lệnh in `https://web.<domain>` (env `base` không có slug trong host: `web.zenify.test`, `be.zenify.test`, `hub.zenify.test`; env khác là `<svc>-<slug>.<domain>`) và email, domain của tài khoản seed. Nếu URL không mở được, [`zenify infra status --local`](/reference/cli/zenify_infra_status) cho biết DNS có resolve không và CA đã được tin chưa.

## Gỡ

[`zenify infra down --local`](/reference/cli/zenify_infra_down) chỉ in preview. Thêm `--apply` để xoá env và platform; thêm `--purge` để gỡ luôn k3s, CA, split DNS và firewall.
