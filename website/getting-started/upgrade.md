---
title: Nâng cấp
---

# Nâng cấp

## Nhắc phiên bản mới ở đầu session

Khi bạn mở một session Claude Code trong workspace, `zenify` kiểm bản mới nhất trên GitHub, tối đa một lần mỗi ngày. Nếu có bản mới hơn, nó in một dòng:

```text
zenify: v0.18.2 is available (running 0.17.4) — upgrade: brew upgrade --cask zenify
```

Dòng nhắc không xuất hiện và không báo lỗi trong ba trường hợp: máy không có mạng, bản đang chạy là bản mới nhất, hoặc bạn build tay (`dev`). Để tắt hẳn, đặt biến `ZENIFY_NO_UPDATE_CHECK=1`.

## Kiểm tra bản mới với `zenify update --check`

```sh
zenify update --help
```

Kết quả:

```text
Tự nhận cách binary này được cài rồi chạy lệnh nâng cấp tương ứng:
  brew            brew upgrade --cask zenify
  scoop           scoop update zenify
  install script  chạy lại scripts/install.sh (install.ps1 trên Windows)

Với --check chỉ báo có bản mới hay không, không cài.

Usage:
  zenify update [flags]

Flags:
      --check   only report whether a newer release exists
  -h, --help    help for update
```

Để chỉ kiểm mà không cài:

```sh
zenify update --check
```

## Nâng cấp với `zenify update`

```sh
zenify update
```

`zenify` nhận ra cách bạn đã cài dựa vào đường dẫn binary: có `Caskroom` hoặc `Cellar` là Homebrew, có `scoop` là Scoop trên Windows, nằm dưới `~/.local/bin` hoặc `%LOCALAPPDATA%\Programs\zenify` là install script. Sau đó nó chạy đúng lệnh nâng cấp cho cách đó. Nếu không nhận ra (`Unknown`), nó in cả ba lệnh nâng cấp để bạn tự chọn và không tự chạy gì.

::: tip Homebrew và Scoop
Homebrew và Scoop quản lý phiên bản qua tap và bucket riêng của kit. `zenify update` chỉ gọi đúng lệnh `brew upgrade --cask zenify` hoặc `scoop update zenify`, không cài lại bằng cách khác.
:::

## Nâng cấp tay một lần cho máy cũ hơn v0.17.7

::: warning
Cơ chế nhắc ở đầu session chỉ có từ `v0.17.7`. Nếu binary trên máy cũ hơn, bạn sẽ không thấy dòng nhắc. Hãy nâng cấp tay một lần bằng Homebrew, Scoop hoặc install script như trong [Cài đặt](/getting-started/install). Từ đó về sau, máy sẽ nhận nhắc cho các bản tiếp theo.
:::

<!-- Nguồn (cho người bảo trì, không hiển thị):
- `zenify update --help`
- `docs/handoff/zenify-kit/update-and-version-gate.md`
- `README.md` (repo `zenify-kit`, mục "Staying current")
-->
