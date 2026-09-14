---
title: Bắt đầu nhanh
---

# Bắt đầu nhanh

Trang này đi qua năm bước, từ khi vừa cài `zenify` đến khi có worktree đầu tiên để code.

```mermaid
flowchart LR
  A["zenify up"] --> B["zenify doctor"] --> C["Kiểm tra skill"] --> D["zenify wt new"]
  class A,B,D action
  class C user
```

*Thứ tự các lệnh khi onboard*

## Bước 1: vào thư mục workspace

Nếu workspace đã tồn tại, `cd` vào đó trước. Nếu chưa có, `zenify up` ở bước 2 sẽ hỏi bạn muốn đặt workspace ở đâu.

```sh
cd ~/Developer/zenify   # macOS mặc định; Linux mặc định ~/zenify, Windows %USERPROFILE%\zenify
```

## Bước 2: onboard workspace với `zenify up`

`zenify up` chạy một wizard tương tác trong terminal theo thứ tự: preflight, đăng nhập `gh`, discover repo, chọn repo, scan, lên kế hoạch, apply và cài hook, verify. Khi không có TTY (CI, script), lệnh in kế hoạch dry-run thay cho wizard. Thêm `--apply` để chạy headless.

```sh
zenify up --help
```

Kết quả:

```text
Onboard workspace: wizard tương tác trong terminal, còn không thì in kế hoạch dry-run (--apply để chạy headless)

Usage:
  zenify up [flags]

Flags:
      --apply              apply changes without the interactive wizard (required for non-interactive/CI runs)
      --dry-run            preview the plan without making changes (default true)
  -h, --help               help for up
      --json               emit the plan as a JSON envelope
      --manifest string    path to repos.yaml (default: manifest/repos.yaml under cwd when present, else the copy embedded in the binary)
      --non-interactive    never prompt — forces the headless dry-run/apply path instead of the interactive wizard
      --overlay string     path to personal overlay (default <workspace>/.zenify-overlay.yaml)
      --workspace string   workspace root (default: the workspace found from cwd or ~/.zenify/workspace; the wizard asks when there is none)
```

Chạy trên một terminal thật:

```sh
zenify up
```

Wizard yêu cầu đăng nhập `gh` nếu bạn chưa đăng nhập. Sau đó nó phát hiện các repo của team và hỏi bạn chọn repo nào để clone hoặc liên kết vào workspace. Từ bước này, các lệnh sau tự tìm workspace qua `~/.zenify/workspace`. Bạn không cần truyền lại `--workspace`.

## Bước 3: kiểm tra môi trường với `zenify doctor`

```sh
zenify doctor --help
```

Kết quả:

```text
Kiểm tra sức khoẻ môi trường, chỉ-đọc (không sửa gì, không in secret; --fix áp tập sửa an toàn)

Usage:
  zenify doctor [flags]

Flags:
      --exit-on-fail   exit non-zero if any check fails
      --fix            apply the safe-subset of automatic repairs, then re-check
  -h, --help           help for doctor
      --json           emit a machine-readable JSON envelope
```

```sh
zenify doctor
```

Kết quả mong đợi: mọi check hiện `OK`. Chi tiết từng check xem [tham chiếu `zenify doctor`](/reference/cli/zenify_doctor). Nếu có check đỏ, chạy `zenify doctor --fix`. Lệnh này chỉ áp tập sửa đã xác nhận an toàn rồi kiểm lại, không đụng gì khác. Đây là nguyên tắc [fail-open](/concepts/gate-fail-open) chung của các gate trong kit.

## Bước 4: kiểm tra skill trong Claude Code

Mở Claude Code trong workspace vừa onboard và gõ:

```
/znf:cook
```

Nếu skill hiện ra trong danh sách gợi ý, plugin `znf:*` đã được cài đúng ở bước 2. Bạn chưa cần chạy `/znf:cook` thật ở bước này.

## Bước 5: tạo worktree đầu tiên

Skill và tài liệu nội bộ của kit viết tắt `wt` thay cho `zenify wt`. Trên máy vừa cài, gõ dạng đầy đủ.

Mọi thay đổi code nằm trong một worktree riêng, không sửa trực tiếp bản checkout chính. Xem [Worktree theo slug](/concepts/worktree-per-slug).

```sh
git fetch origin
zenify wt new demo --type feat --base origin/<base>
```

`<base>` là branch gốc khai báo trong `.claude/worktree.json` của repo đó, ví dụ `staging` hoặc `main`. Giá trị này khác nhau theo repo. Lệnh tạo một worktree mới ở `.worktrees/demo` với branch, port và `.env` riêng.

Vì đây là demo, xoá worktree ngay:

```sh
zenify wt rm demo
```

`zenify wt rm` từ chối xoá một worktree chưa có dấu vết merge. Với worktree demo chưa commit gì, lệnh xoá được ngay.

## Bước tiếp theo

Khi có task thật, xem [Chọn workflow](/workflows/) để biết nên dùng cook, fix hay hotfix.

<!-- Nguồn (cho người bảo trì, không hiển thị):
- `zenify up --help`, `zenify doctor --help`
- `docs/handoff/zenify-kit/onboarding-tui.md` (luồng wizard Preflight, Identity, Discover, Select, Scan, Plan, Apply, Verify/Done)
- `README.md` (repo `zenify-kit`, mục "After install")
-->
