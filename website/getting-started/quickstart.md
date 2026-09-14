---
title: Bắt đầu nhanh
---

# Bắt đầu nhanh

Năm bước từ "vừa cài xong `zenify`" đến "có worktree đầu tiên để code".

```mermaid
flowchart LR
  A[zenify up] --> B[zenify doctor] --> C[wt new]
```

## Bước 1 — vào thư mục workspace

`zenify up` (bước 2) tự hỏi bạn muốn đặt workspace ở đâu nếu chưa có; nếu workspace đã tồn tại, `cd` vào đó trước.

```sh
cd ~/Developer/zenify   # macOS mặc định; Linux mặc định ~/zenify, Windows %USERPROFILE%\zenify
```

## Bước 2 — `zenify up`: onboard workspace

`zenify up` chạy một wizard tương tác trong terminal (preflight → đăng nhập `gh` → discover repo → chọn repo → scan → lên kế hoạch → apply + cài hook → verify). Không có TTY (CI, script), nó in kế hoạch dry-run thay vì mở wizard; thêm `--apply` để chạy headless thật sự.

```sh
zenify up --help
```

Kết quả thật (ground trên binary build từ commit 592f7a4):

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

Wizard hỏi bạn đăng nhập `gh` (nếu chưa), sau đó tự phát hiện các repo của team và hỏi bạn chọn repo nào để clone/liên kết vào workspace. Xong bước này, các lệnh sau tự tìm ra workspace qua `~/.zenify/workspace` — không cần chỉ định lại `--workspace`.

## Bước 3 — `zenify doctor`: kiểm tra môi trường

```sh
zenify doctor --help
```

Kết quả thật (ground trên binary build từ commit 592f7a4):

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

Kết quả mong đợi: mọi check hiện `OK` (xem chi tiết từng lệnh ở [tham chiếu `zenify doctor`](/reference/cli/zenify_doctor)). Nếu có check đỏ, thử `zenify doctor --fix` — nó chỉ áp tập sửa được xác nhận an toàn rồi kiểm lại, không đụng gì khác<!-- TODO(Task 8): link /concepts/gate-fail-open -->.

## Bước 4 — mở Claude Code, kiểm tra skill có mặt

Mở Claude Code trong workspace vừa onboard và gõ:

```
/znf:cook
```

Nếu skill hiện ra trong danh sách gợi ý, plugin `znf:*` đã nối dây đúng (`zenify up` đã cài nó ở bước 2). Bạn chưa cần chạy `/znf:cook` thật — chỉ cần thấy nó xuất hiện.

## Bước 5 — worktree đầu tiên: `wt new` rồi `wt rm`

Mọi thay đổi code luôn nằm trong một worktree riêng, không sửa trực tiếp bản checkout chính<!-- TODO(Task 8): link /concepts/worktree-per-slug -->.

```sh
git fetch origin
wt new demo --type feat --base origin/<base>
```

`<base>` là nhánh gốc khai báo trong `.claude/worktree.json` của repo đó (ví dụ `staging` hoặc `main` — khác nhau theo repo). Lệnh tạo một worktree mới ở `.worktrees/demo`, với branch, port và `.env` riêng.

Dọn thử ngay vì đây chỉ là demo:

```sh
wt rm demo
```

`wt rm` từ chối xoá một worktree chưa có dấu vết merge — với một worktree demo chưa commit gì, nó xoá được ngay.

## Bước tiếp theo

Có task thật rồi? Chọn quy trình cook / fix / hotfix<!-- TODO(Task 9): link /workflows/ -->.

## Nguồn

- Ground trên binary build từ commit 592f7a4 của nhánh này (2026-09-14), chưa phát hành: `zenify up --help`, `zenify doctor --help`.
- `docs/handoff/zenify-kit/onboarding-tui.md` (luồng wizard Preflight → Identity → Discover → Select → Scan → Plan → Apply → Verify/Done)
- `README.md` (repo `zenify-kit`, mục "After install")
