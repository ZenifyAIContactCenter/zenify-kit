---
title: Onboard workspace và repo
---

# Onboard workspace và repo

Trang này gom lại các lệnh dùng khi đưa một máy mới vào workspace, hoặc khi sửa lại layout repo trên một workspace đã có. Với luồng đầy đủ từng bước, đọc [Bắt đầu nhanh](/getting-started/quickstart) trước.

## Khi nào dùng

Dùng các lệnh ở đây khi cài ZenifyKit trên một máy mới, khi team thêm repo và bạn cần kéo về đúng bộ, khi cần kiểm tra môi trường đang thiếu gì, hoặc khi một workspace cũ có repo nằm rải ở gốc và cần chuyển sang layout `repos/<tên>`.

## Các bước

```mermaid
flowchart TD
  A["zenify up"] --> B["zenify doctor"]
  B -->|Có mục hỏng| C["zenify doctor --fix"]
  C --> B
  B -->|Mọi mục đạt| D["Bắt đầu làm việc"]
  class A,B,C action
  class D user
```

*Trình tự onboard một máy mới*

`zenify up` chạy an toàn nhiều lần: mỗi lần chạy lại nó chỉ sửa những gì lệch so với cấu hình chung của team, không ghi đè việc bạn đã tự chỉnh.

## Lệnh

### `zenify up`

Onboard máy vào workspace. Chạy trong terminal, lệnh mở wizard: chọn thư mục workspace, chọn repo bạn có quyền truy cập, rồi, với manifest không khai báo `broker:`, nhập các key toàn cục vào `<workspace>/.zenify/.env`. Khi manifest khai báo `broker:`, credential DB, E2E và PostHog đi qua broker và wizard không hỏi các key đó; xem [Dùng credential broker](/guides/broker-client).

Khi apply, lệnh clone repo còn thiếu, ghi các file cấu hình do kit sở hữu, gắn hook `znf` vào `~/.claude/settings.json`, đồng bộ plugin skill, chuẩn bị Playwright nếu workspace có repo frontend, cài extension `gh-stack` nếu thiếu (`gh extension install github/gh-stack`, lỗi chỉ cảnh báo), clone knowledge store và ghi con trỏ workspace vào `~/.zenify/workspace`. Với Playwright, lệnh đăng ký MCP ở user scope kèm `--isolated` (mỗi session một trình duyệt riêng trong bộ nhớ, nên nhiều session chạy song song không giành nhau profile), `--caps=storage` và thư mục output `~/.zenify/playwright`; gói MCP ghim phiên bản (`@playwright/mcp@0.0.83`) và Chromium được cài qua đúng gói đó, nên trình duyệt chỉ tải lại khi kit nâng pin, tiến trình tải hiện ngay trên terminal; khi đúng bản Chromium đó đã cài xong, lệnh bỏ qua bước cài và in `chromium <revision> already installed`, đồng thời thêm vào `permissions.allow` của `~/.claude/settings.json` các rule cho tool `mcp__playwright__*` mà `znf:ui-verifier` dùng, cùng `Bash(zenify e2e *)` và `Bash(zenify visual *)` và `Bash(zenify ui-verify *)`, nên verify UI không hỏi quyền. Lệnh cũng thêm `Bash(zenify db-read *)` và `Bash(zenify redis-read *)` (chỉ đọc) vào `permissions.allow`, và `zenify db-read *`, `zenify redis-read *` vào `sandbox.excludedCommands` vì chúng gọi broker; `zenify data run` không được thêm, vẫn hỏi quyền. Lệnh cũng thêm vào `permissions.allow` cho stacked PR đúng các lệnh `gh stack` mà skill dùng: `Bash(gh stack --help)` và `Bash(gh stack <lệnh> *)` với `init`, `add`, `checkout`, `rebase`, `push`, `submit`, `view`; lệnh `gh stack` khác vẫn hỏi quyền. Lệnh cũng thêm `zenify e2e login *` vào `sandbox.excludedCommands`, vì sandbox Bash chặn lệnh này ghi `~/.zenify` và gọi app local; các lệnh `zenify e2e` khác vẫn chạy trong sandbox. Tương tự, `gh stack <lệnh> *` với bảy lệnh đó cũng được thêm vào `sandbox.excludedCommands` vì chúng đọc auth của `gh` và gọi GitHub, cả hai đều bị sandbox chặn; lệnh `gh stack` khác vẫn chạy trong sandbox, và git-guard vẫn chặn merge, gỡ, ghép stack và `gh stack alias` trước mọi rule quyền. Khi bạn không bật sandbox, mục này không có tác dụng. Đăng ký MCP cũ của kit được chuyển sang dạng mới; đăng ký bạn đã tự chỉnh được giữ nguyên và lệnh cảnh báo. `zenify down` chỉ gỡ các mục kit đã thêm, mục bạn có từ trước vẫn còn; luật deny `.zenify/.env` của workspace đó do kit thêm bị gỡ khỏi `~/.claude/settings.json`, luật deny bạn tự thêm và luật của workspace khác giữ nguyên. Mục kit mà bạn đã tự xoá khỏi `permissions.allow` hoặc `sandbox.excludedCommands` sẽ không bị thêm lại. Lệnh cần `gh` đã đăng nhập với scope `read:org` và `repo`.

Credential nằm ở `<workspace>/.zenify/.env`, không còn là placeholder trong khối `env` của settings; `zenify up --apply` dựng file thành khung có sẵn mọi key của manifest (key trống là `KEY=`), chia mục Staging, Tooling, Production, Custom; wizard chỉ hỏi các key toàn cục, bỏ nhóm production. Key không có trong manifest, ví dụ `OPENROUTER_API_KEY`, là key custom, thêm bằng `zenify env set OPENROUTER_API_KEY --desc "key cá nhân"`. `zenify up --apply` chuyển key manifest còn nằm trong khối `env` của `settings.local.json` sang file mới và thêm luật deny `Read(//<workspace>/.zenify/.env)` (đường dẫn tuyệt đối) vào `~/.claude/settings.json`. Chi tiết và quy tắc khi hai giá trị khác nhau: [Credential dev với zenify env](/guides/env).

```bash
zenify up
zenify up --non-interactive --dry-run --workspace ~/WorkingSpace/zenify
zenify up --non-interactive --apply --workspace ~/WorkingSpace/zenify
```

| Cờ | Ý nghĩa |
|---|---|
| `--apply` | Thực thi kế hoạch. Không kèm cờ này và không ở terminal, lệnh chỉ in kế hoạch |
| `--dry-run` | Chỉ in kế hoạch, không đổi gì. Mặc định khi chạy ngoài terminal |
| `--non-interactive` | Không mở wizard kể cả khi đang ở terminal. Dùng trong script |
| `--workspace` | Thư mục workspace |

### `zenify down`

Gỡ phần ZenifyKit đã cài vào máy và repo, giữ nguyên code, workspace và knowledge store. Đây là bước đảo của `zenify up`. Mặc định chỉ in preview, thêm `--apply` để thực thi. Lệnh không đụng vào repo đã clone hay knowledge store.

```bash
zenify down
zenify down --apply
```

### `zenify doctor`

Kiểm tra sức khỏe môi trường và báo từng mục đạt hay hỏng: phiên bản binary, đăng nhập git và quyền GitHub, danh tính commit của git (`user.name`, `user.email`), credential trong `.zenify/.env` (`secrets` báo theo tên key toàn cục (trừ nhóm production) có giá trị hay trống, `env-file` báo quyền rộng hơn 0600 và key custom chưa có mô tả, `settings-env` báo theo tên key còn nằm trong settings), mục `broker-creds` (xin thử credential staging từ broker, báo tên biến nhận được; bỏ qua khi không có broker), tool bên ngoài (`git`, `gh`, `mongosh`, `mysql`), Playwright (gồm đăng ký MCP, báo nếu bạn dùng đăng ký tuỳ biến), quyền Playwright và `zenify e2e`/`zenify visual`/`zenify ui-verify` trong `permissions.allow`, mục `gh-stack` (cần `gh` từ 2.90.0 và extension `github/gh-stack`; `zenify doctor --fix` cài extension nếu thiếu bằng `gh extension install github/gh-stack`), Docker và plugin skill `znf`. Chạy ngay sau `zenify up`, và mỗi khi một skill báo thiếu tool hoặc thiếu secret. Lệnh chỉ đọc, chạy bao nhiêu lần cũng được.

```bash
zenify doctor
zenify doctor --fix
```

| Cờ | Ý nghĩa |
|---|---|
| `--fix` | Tự sửa các mục an toàn rồi kiểm tra lại. Mục cần bạn tự làm vẫn in hướng dẫn |
| `--exit-on-fail` | Trả mã thoát khác 0 khi có mục hỏng, dùng trong script và CI |
| `--json` | In kết quả dạng JSON |

### `zenify migrate`

Gom các repo đang nằm rải ở gốc workspace vào thư mục con `repos/<tên>`, layout mà `zenify up` dùng. Chạy một lần khi workspace cũ chưa theo layout này.

```bash
zenify migrate
zenify migrate --apply
```

Mặc định lệnh chỉ liệt kê từng repo với hành động `Move`, `Refuse` hoặc `Skip` kèm lý do. Với `--apply`, lệnh di chuyển repo, sửa lại liên kết worktree, trỏ lại symlink `node_modules` và cập nhật đường dẫn trong manifest workspace, rồi nhắc bạn khởi động lại dev server đang chạy. Repo còn thay đổi chưa commit hoặc có worktree đang mở bị đánh `Refuse`.

### `zenify update`

Nâng cấp binary lên bản mới nhất bằng đúng kênh đã cài (Homebrew, Scoop, hoặc script cài đặt), hoặc chỉ kiểm tra có bản mới với `--check`. Đầu mỗi phiên, binary tự in dòng `zenify: vX is available` khi có bản mới; đặt `ZENIFY_NO_UPDATE_CHECK=1` để tắt dòng nhắc này.

```bash
zenify update --check
zenify update
```

## Cấu hình worktree của một repo

Mỗi repo tham gia workspace khai báo một file `.claude/worktree.json`, ví dụ của `zenify-kit`:

```json
{
  "abbrev": "kit",
  "baseRef": "origin/main",
  "worktreeDir": ".worktrees/",
  "copy": [".claude/settings.local.json"],
  "deps": "none",
  "portEnv": "PORT",
  "portRange": [3750, 3799]
}
```

| Trường | Ý nghĩa |
|---|---|
| `abbrev` | tên viết tắt của repo |
| `baseRef` | branch gốc mặc định cho `zenify wt new --type feat/fix` |
| `hotfixBaseRef` | base ref riêng cho `--type hotfix`, nếu khác `baseRef` |
| `worktreeDir` | thư mục chứa các worktree, thường `.worktrees/` |
| `copy` | các file được seed từ checkout chính vào worktree mới, ví dụ `.claude/settings.local.json` và `CLAUDE.md`. Nhờ seed `.claude/settings.local.json`, worktree dùng chung secret và bộ nhớ với checkout chính, xem [Secret và settings.local.json](/guides/read-real-data#secret-va-settings-local-json) |
| `deps` | cách seed dependency: `symlink`, `clone`, hoặc `none` cho repo không phải Node |
| `install` | lệnh cài dependency khi `deps` cần cài riêng |
| `portEnv` | tên biến môi trường worktree đọc port từ đó |
| `portRange` | khối port riêng của repo, không trùng repo khác trong workspace |

Chi tiết cách chọn `baseRef`, `hotfixBaseRef` và dải port cho repo mới: [Base ref và port block](/concepts/base-ref-and-ports).

## Kết quả

`.worktrees/` phải nằm trong `.gitignore` đã commit của mỗi repo, không nằm trong `.git/info/exclude`. Khai báo cục bộ không đi theo repo, nên đồng nghiệp clone lần đầu sẽ thấy checkout bị bẩn ngay khi chạy `zenify wt new`.

## Lưu ý

- `zenify up` chạy lại được bất cứ lúc nào, kể cả sau khi team thêm repo mới.
- Repo không phải Node vẫn onboard bình thường, chỉ cần `.claude/worktree.json` khai `"deps": "none"` và một khối port riêng.

## Xem thêm

[`zenify up`](/reference/cli/zenify_up), [`zenify doctor`](/reference/cli/zenify_doctor), [`zenify migrate`](/reference/cli/zenify_migrate), [`zenify down`](/reference/cli/zenify_down), [`zenify update`](/reference/cli/zenify_update), [Bắt đầu nhanh](/getting-started/quickstart), [Base ref và port block](/concepts/base-ref-and-ports)

<!-- Nguồn (cho người bảo trì, không hiển thị):
- reference/cli/zenify_up.md, zenify_down.md, zenify_doctor.md, zenify_migrate.md, zenify_update.md (generated)
- concepts/base-ref-and-ports.md (worktree.json field example và ý nghĩa)
-->
