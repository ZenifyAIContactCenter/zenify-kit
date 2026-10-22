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

Lệnh in ra số phiên bản đang cài, ví dụ:

```text
v0.18.2
```

Bản build tay từ mã nguồn in `dev` thay cho số phiên bản.

::: warning Máy có bản cũ hơn v0.17.7
Cơ chế nhắc phiên bản mới ở đầu mỗi session chỉ có từ `v0.17.7`. Nếu binary trên máy cũ hơn, bạn cần nâng cấp tay một lần bằng `brew upgrade --cask zenify`, `scoop update zenify`, hoặc chạy lại install script. Từ đó về sau, `zenify update` xử lý các lần nâng cấp tiếp theo.
:::

## Lệnh `wt` và `pm`

`zenify up --apply` cài hai shim `~/.local/bin/wt` và `~/.local/bin/pm` (mode 0755), mỗi file chạy `zenify wt` và `zenify pm`, để skill và worker chạy nền gọi được `wt` và `pm` mà không cần gõ `zenify`. `pm` chọn đúng package manager JS của project: trường `packageManager` của `package.json` gần nhất, không có thì theo lockfile (`pnpm-lock.yaml`, `yarn.lock`, `package-lock.json`, `bun.lock`/`bun.lockb`). Trong monorepo, package con không có cả hai thì `pm` lấy tín hiệu của thư mục cha gần nhất có một trong hai, tới git toplevel, và vẫn chạy manager tại thư mục của package con. `packageManager` chỉ nhận `pnpm`, `yarn`, `npm`, `bun`; tên khác bị từ chối kèm giá trị. Chạy `pm` không đối số thì in tên manager ra stdout, kèm `root` và `reason` ra stderr:

```text
$ cd website && pm
npm
root: /…/zenify-kit/website
reason: package-lock.json
```

Mỗi shim có dòng `# managed by zenify`. File `wt` hay `pm` bạn tự viết (không có dòng này) được giữ nguyên: `zenify up` chỉ cảnh báo `… exists and is not managed by zenify; left unchanged`. `zenify down --apply` gỡ các shim có dấu này và để file của bạn yên.

`zenify doctor` có mục `shims`, đòi `wt` và `pm` resolve được trên `PATH`. Thiếu thì in `shims   wt=missing pm=missing — add ~/.local/bin to PATH`; đủ thì in `shims   wt=ok pm=ok`.

Sau khi cài, đi tiếp tới [Bắt đầu nhanh](/getting-started/quickstart) để onboard workspace.

<!-- Nguồn (cho người bảo trì, không hiển thị):
- `README.md` (repo `zenify-kit`, mục "Install")
- Nội dung cảnh báo lấy từ tag `v0.17.7` (`git show v0.17.7`) và `update-and-version-gate.md`, không phải từ `--help`
-->
