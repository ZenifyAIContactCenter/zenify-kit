---
title: Nâng cấp
---

# Nâng cấp

## Nhắc tự động ở đầu session

Khi bạn mở một session Claude Code trong workspace, `zenify` kiểm bản mới nhất trên GitHub (tối đa một lần mỗi ngày) và in một dòng nếu có bản mới hơn:

```text
zenify: v0.18.0 is available (running 0.17.4) — upgrade: brew upgrade --cask zenify
```

Không có mạng, hoặc bản đang chạy là bản mới nhất, hoặc bạn build tay (`dev`) — dòng nhắc này im lặng, không báo lỗi. Tắt hẳn nhắc bằng biến `ZENIFY_NO_UPDATE_CHECK=1`.

## `zenify update --check`: chỉ kiểm, không cài

```sh
zenify update --help
```

Kết quả thật (ground trên binary build từ commit 592f7a4):

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

```sh
zenify update --check
```

## `zenify update`: nâng cấp thật

```sh
zenify update
```

`zenify` tự nhận ra bạn cài bằng cách nào (đường dẫn binary có `Caskroom`/`Cellar` → Homebrew; có `scoop` → Scoop trên Windows; nằm dưới `~/.local/bin` hoặc `%LOCALAPPDATA%\Programs\zenify` → install script), rồi chạy đúng lệnh nâng cấp cho cách đó. Nếu không nhận ra cách cài (`Unknown`), nó in cả ba lệnh nâng cấp để bạn tự chọn, không tự chạy gì.

::: tip Brew vs scoop
Homebrew và Scoop tự quản lý phiên bản qua tap/bucket riêng của kit — `zenify update` chỉ gọi đúng lệnh `brew upgrade --cask zenify` hoặc `scoop update zenify` cho bạn, không cài lại từ đầu bằng cách khác.
:::

## Nâng cấp tay một lần (máy cũ hơn v0.17.7)

::: warning
Cơ chế nhắc tự động ở đầu session chỉ có kể từ `v0.17.7`. Nếu binary trên máy bạn cũ hơn, bạn sẽ không thấy dòng nhắc — phải tự nâng cấp tay **một lần** (Homebrew/Scoop/install script như [Cài đặt](/getting-started/install)) để bắt đầu nhận nhắc cho các bản sau này.
:::

## Nguồn

- Ground trên binary build từ commit 592f7a4 của nhánh này (2026-09-14), chưa phát hành: `zenify update --help`.
- `docs/handoff/zenify-kit/update-and-version-gate.md`
- `README.md` (repo `zenify-kit`, mục "Staying current")
