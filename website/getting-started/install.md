---
title: Cài đặt
---

# Cài đặt

Bạn có ba cách cài `zenify`: install script, Homebrew (macOS và Linux), hoặc Scoop (Windows). Cả ba cài cùng một binary. Chúng chỉ khác ở cách nâng cấp về sau, xem [Nâng cấp](/getting-started/upgrade).

## Install script (khuyến nghị)

Cách này chỉ cần một dòng lệnh và không cần trình quản lý gói. Script kiểm checksum SHA-256 của bản tải về trước khi cài. Trên macOS, binary không bị Gatekeeper quarantine vì được tải bằng curl, không qua trình duyệt.

macOS / Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.ps1 | iex
```

Script cài vào `~/.local/bin/zenify` trên macOS và Linux, hoặc `%LOCALAPPDATA%\Programs\zenify` trên Windows. Vị trí Windows cố định trong script. Để đổi vị trí cài trên macOS và Linux, đặt biến `ZENIFY_BIN`. Script Windows không đọc biến này.

Để ghim một phiên bản cụ thể, đặt `ZENIFY_VERSION=v0.5.0`. Cách này dùng được trên cả hai hệ điều hành.

Script tự thêm `zenify` vào `PATH`, cài `gh` (GitHub CLI) nếu máy chưa có, và cài các skill `znf:*`.

## Homebrew (macOS và Linux)

```sh
brew tap zenifyaicontactcenter/zenify-kit https://github.com/ZenifyAIContactCenter/zenify-kit
brew trust zenifyaicontactcenter/zenify-kit   # một lần: Homebrew yêu cầu trust một tap bên thứ ba
brew install --cask zenify
```

Nâng cấp về sau bằng `brew upgrade --cask zenify`.

## Scoop (Windows)

```powershell
scoop bucket add zenify https://github.com/ZenifyAIContactCenter/zenify-kit
scoop install zenify
```

## Kiểm tra cài đặt

```sh
zenify version
```

Với bản phát hành mới nhất tại thời điểm viết, lệnh in:

```text
v0.17.7
```

Bản build tay từ mã nguồn in `dev` thay cho số phiên bản.

::: warning Máy có bản cũ hơn v0.17.7
Cơ chế nhắc phiên bản mới ở đầu mỗi session chỉ có từ `v0.17.7`. Nếu binary trên máy cũ hơn, bạn cần nâng cấp tay một lần bằng `brew upgrade --cask zenify`, `scoop update zenify`, hoặc chạy lại install script. Từ đó về sau, `zenify update` xử lý các lần nâng cấp tiếp theo.
:::

Sau khi cài, đi tiếp tới [Bắt đầu nhanh](/getting-started/quickstart) để onboard workspace.

<!-- Nguồn (cho người bảo trì, không hiển thị):
- `README.md` (repo `zenify-kit`, mục "Install")
- `docs/handoff/zenify-kit/update-and-version-gate.md` (ghi chú v0.17.7, máy từ v0.17.6 trở xuống cần nâng cấp tay một lần)
- Nội dung cảnh báo lấy từ tag `v0.17.7` (`git show v0.17.7`) và `update-and-version-gate.md`, không phải từ `--help`
-->
