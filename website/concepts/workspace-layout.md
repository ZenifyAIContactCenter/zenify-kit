---
title: Quy ước thư mục workspace
---

# Quy ước thư mục workspace

## Tổng quan

Mọi máy đã `zenify up` dùng chung một quy ước: root workspace chỉ có một bộ entry cố định, `docs/` chỉ có mười tên, và hook `write-guard` từ chối ghi ra ngoài quy ước. Mục đích là rác không được sinh ra ngay từ đầu, thay vì phải dọn sau.

**Root workspace chỉ chứa:** `repos/`, `docs/`, `CLAUDE.md`, `AGENTS.md`, `.claude/`, `.zenify/`, `.zenify-overlay.yaml`, và của editor/hệ thống `.git/`, `.idea/`, `.vscode/`, `.DS_Store`. Mọi entry khác là vi phạm.

**`docs/` chỉ chứa mười tên.** Năm của store (symlink, sync): `specs`, `plans`, `reference`, `handoff`, `releases`. Năm local (thư mục thật, không git): `research`, `notes`, `scripts`, `exports`, `external`. Không file nào nằm ngay dưới `docs/`. (`docs/.claude/` do harness tạo, bỏ qua như mọi dot-entry.)

## Bảng quy ước

| Loại file | Ai sinh | Chỗ | Sync | Ai dọn |
|---|---|---|---|---|
| File tạm, script thử, ảnh kiểm tra nhanh, output trung gian | agent | scratchpad `$TMPDIR` của session | không | harness |
| Spec | brainstorming | `docs/specs/<repo>/` | có | không bao giờ xóa |
| Plan | writing-plans | `docs/plans/<repo>/` | có (speclife cần `hasPlan`; `/cook <plan>` resume từ máy khác) | `docs sweep` khi spec `built`/`superseded` → `docs/notes/plans/<repo>/` |
| Research nháp | `znf:research` | `docs/research/<repo>/` | không | người dùng |
| Research được spec trỏ tới | brainstorming `mv` | `docs/reference/<repo>/` | có | `docs sweep` khi không spec/plan/.config nào trỏ → `docs/research/<repo>/` |
| Ghi chú phiên, handoff cho chính mình | finishing-a-development-branch | `docs/notes/` | không | người dùng |
| Handoff cho người khác | finishing-a-development-branch | `docs/handoff/` | có | `docs sweep` sau 30 ngày → `docs/notes/` |
| Script cho người chạy (sửa data, backfill, `--dry-run`) và json kết quả | agent | `docs/scripts/<repo>/` | không | người dùng |
| Sản phẩm cho người đọc: docx, xlsx, pdf, báo cáo, hình minh họa | agent | `docs/exports/` | không (gửi đi bằng kênh khác) | người dùng |
| Tài liệu bên ngoài người đưa cho agent đọc | người dùng | `docs/external/` | không | người dùng |
| Ảnh, snapshot, console log của Playwright | `zenify browser` | `~/.zenify/playwright/sessions/<phiên>/.playwright-cli/` | không | người dùng |
| Release report | kit M7 | `docs/releases/` | có | không đụng |
| Ledger SDD, ship board, review log, e2e, visual | kit | `<repo>/.znf/` (gitignored), `$TMPDIR` | không | kit |
| Code | agent | worktree `<repo>/.worktrees/<slug>/` | PR | `wt sweep` |

`zenify doctor` có hai check báo vi phạm: `workspace-root` (entry ngoài quy ước ở root và dưới `docs/`) và `memory-index` (`MEMORY.md` gần trần nạp mỗi phiên). Cả hai chỉ báo, không có lệnh dọn root. Danh sách repo cho release report nằm ở `.zenify/release-repos.txt`; `.znf/` ở root cũ vẫn được đọc.

## Hook write-guard

Hook `PreToolUse` trên Write, Edit và NotebookEdit. Nó từ chối ghi vào root workspace ngoài danh sách cho phép, vào file ngay dưới `docs/`, hoặc vào `docs/<tên>` ngoài mười tên. Hook chỉ chạy trên Claude; Codex không có hook cho thao tác Write. Ảnh Playwright không đi qua hook này: `zenify browser` chạy từ thư mục phiên dưới `~/.zenify/playwright/sessions/` và in đường dẫn tuyệt đối, nên ảnh không rơi vào repo.

Khi bị chặn, agent thấy message (exit 2):

```
write-guard: <path> is outside the workspace layout. temp → $TMPDIR · research → docs/research/<repo>/ · notes → docs/notes/ · script for the user → docs/scripts/<repo>/ · deliverable → docs/exports/ · external document → docs/external/ · code → repos/<repo>/.worktrees/<slug>/
```

Cách đọc: `<path>` là đích agent định ghi. Chọn đúng một chỗ trong danh sách theo loại file (cùng bảng ở trên) và ghi lại ở đó. Hook fail-open: lỗi bất ngờ không chặn phiên, chỉ deny đúng vi phạm quy ước.

## Tìm file đã bị sweep xóa

`zenify docs sweep` (mặc định chỉ in kế hoạch, `--apply` mới xóa) chép mỗi file bị xóa sang dir local trước khi xóa: reference → `docs/research/<repo>/`, handoff và plan → `docs/notes/`. Tìm ở đó trước. Bản gốc trong store vẫn còn trong git history:

```bash
git -C ~/.zenify/knowledge log --all -- <path>
```

Dùng lệnh này chỉ để đọc lịch sử; đừng sửa gì trong store bằng git.

## Liên quan

- [Knowledge store và docs/](/concepts/knowledge-store): store, view symlink và `docs sync`.
- [Research có kiểm chứng](/concepts/research): kết quả research ghi ở `docs/research/<repo>/`.

<!-- Nguồn (cho người bảo trì, không hiển thị):
- `docs/specs/zenify-kit/2026-10-12-knowledge-hygiene-design.md` (mục "Quy ước thư mục", bảng nguyên văn)
- `internal/cli/writeguard.go` (message deny), `internal/apply/globalhooks.go` (matcher), `internal/cli/doctor_context.go`
-->
