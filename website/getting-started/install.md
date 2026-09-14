---
title: Cài đặt
---

# Cài đặt

Bạn có ba cách cài `zenify`: install script (khuyến nghị), Homebrew (macOS/Linux), hoặc Scoop (Windows). Cả ba đều cài cùng một binary, chỉ khác cơ chế nâng cấp sau này — xem [Nâng cấp](/getting-started/upgrade).

## Install script (khuyến nghị)

Một dòng lệnh, không cần trình quản lý gói. Script kiểm checksum SHA-256 của bản tải về trước khi cài, và trên macOS tự gỡ Gatekeeper quarantine để binary chạy được ngay.

**macOS / Linux**

```sh
curl -fsSL https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.ps1 | iex
```

Cài vào `~/.local/bin/zenify` (macOS/Linux) hoặc `%LOCALAPPDATA%\Programs\zenify` (Windows). Đổi vị trí cài bằng biến `ZENIFY_BIN`; ghim một phiên bản cụ thể bằng `ZENIFY_VERSION=v0.5.0`. Script tự thêm `zenify` vào `PATH`, cài `gh` (GitHub CLI) nếu máy chưa có, và nối dây các skill `znf:*`.

## Homebrew (macOS + Linux)

```sh
brew tap zenifyaicontactcenter/zenify-kit https://github.com/ZenifyAIContactCenter/zenify-kit
brew trust zenifyaicontactcenter/zenify-kit   # một lần: Homebrew yêu cầu trust một tap bên thứ ba
brew install --cask zenify
```

Nâng cấp sau này bằng `brew upgrade --cask zenify`.

## Scoop (Windows)

```powershell
scoop bucket add zenify https://github.com/ZenifyAIContactCenter/zenify-kit
scoop install zenify
```

## Kiểm tra cài đặt thành công

```sh
zenify version
```

Kết quả mong đợi trên máy vừa cài bản phát hành mới nhất tại thời điểm viết (v0.17.7):

```text
v0.17.7
```

Trên một bản build cục bộ không qua release (`go build`), lệnh này in `dev` thay vì số phiên bản.

::: warning
Nếu binary trên máy bạn cũ hơn `v0.17.7`, bạn cần nâng cấp tay **một lần** (`brew upgrade --cask zenify` / `scoop update zenify` / chạy lại install script) trước khi thông báo nhắc phiên bản mới ở đầu mỗi session hoạt động — cơ chế nhắc tự động chỉ có kể từ `v0.17.7`. Từ đó về sau `zenify update` xử lý mọi lần nâng cấp tiếp theo.
:::

Sau khi cài, đi tiếp tới [Bắt đầu nhanh](/getting-started/quickstart) để onboard workspace.

## Nguồn

- `README.md` (repo `zenify-kit`, mục "Install")
- `docs/handoff/zenify-kit/update-and-version-gate.md` (ghi chú v0.17.7, máy ≤v0.17.6 cần nâng tay một lần)
- Cảnh báo "≤v0.17.6 cần nâng tay một lần" lấy từ nội dung tag `v0.17.7` (`git show v0.17.7`) và `update-and-version-gate.md` — không phải từ `--help`.
- Ground trên binary build từ commit 592f7a4 của nhánh này (2026-09-14), chưa phát hành.
