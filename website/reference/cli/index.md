---
title: Lệnh CLI
---

# Lệnh CLI

Trang này và mọi trang con được sinh bởi `zenify docs gen` từ chính binary; không sửa tay.

| Lệnh | Mô tả |
|---|---|
| [zenify analyze](./zenify_analyze) | Phân tích cơ học cặp spec+plan (coverage, marker, cấu trúc Brief) — advisory, fail-open |
| [zenify config](./zenify_config) | phân phối config workspace-level từ docs/.config (dry-run mặc định) |
| [zenify db-perf](./zenify_db-perf) | Quét tĩnh một diff tìm anti-pattern query (hai tầng) — advisory, fail-open |
| [zenify db-read](./zenify_db-read) | Đọc chỉ-đọc các database dùng chung của zenify (không bao giờ ghi, không in secret) |
| [zenify docs](./zenify_docs) | quản lý docs layer (agent-managed, dev read-only) |
| [zenify docs gen](./zenify_docs_gen) | sinh reference cho site docs (CLI từ cobra, skill/agent từ frontmatter, bảng hook) |
| [zenify docs sync](./zenify_docs_sync) | đồng bộ docs: pull + commit + push (fail-open) |
| [zenify doctor](./zenify_doctor) | Kiểm tra sức khoẻ môi trường, chỉ-đọc (không sửa gì, không in secret; --fix áp tập sửa an toàn) |
| [zenify down](./zenify_down) | Offboard: gỡ znf global hooks, .worktrees/ + .wt/ excludes, và owned settings skeletons (preview mặc định; --apply để thực thi) |
| [zenify e2e](./zenify_e2e) | E2E functional (Playwright journey thật trong Docker) + lint chống test hợt |
| [zenify e2e lint](./zenify_e2e_lint) | Chặn cơ học journey 'hợt' (thiếu re-fetch/assert/cleanup, dùng anti-pattern) |
| [zenify e2e run](./zenify_e2e_run) | Chạy journey .znf/e2e trong Docker (cần --port dev-server host) |
| [zenify gate](./zenify_gate) | trợ giúp gate (contract sweep) |
| [zenify gate participants](./zenify_gate_participants) | liệt kê repo tham gia contract gate (lưu ở gate-participants.json + worktree.json gate.sharedStore=true) |
| [zenify guard](./zenify_guard) | Quản lý git-guard hook |
| [zenify guard install](./zenify_guard_install) | Đăng ký PreToolUse hook trỏ `zenify git-guard` trong ~/.claude/settings.json |
| [zenify hotfix](./zenify_hotfix) | trợ giúp hotfix |
| [zenify hotfix baseref](./zenify_hotfix_baseref) | in base ref hotfix đã resolve theo chiến lược trong worktree.json |
| [zenify migrate](./zenify_migrate) | gom repo vào một thư mục con (dry-run mặc định; move-and-repair: gom được cả repo dirty và repo còn worktree) |
| [zenify observe](./zenify_observe) | Observability: đếm/nhắc fan-out subagent |
| [zenify observe report](./zenify_observe_report) | Tóm tắt observe theo session (dispatch + tool-output) |
| [zenify observe statusline](./zenify_observe_statusline) | HUD statusline: hiện model · ctx% · ⟳dispatch · ↓tool-output · $cost |
| [zenify observe statusline install](./zenify_observe_statusline_install) | Ghi key statusLine → `zenify observe statusline` vào ~/.claude/settings.json (chỉ khi trống) |
| [zenify release-note](./zenify_release-note) | ghi note-commit release (trailer risk-metadata) — dùng bởi /ship |
| [zenify release-report](./zenify_release-report) | sinh report rủi ro cho một release (chỉ-đọc, ghi docs/releases/R<N>.md) |
| [zenify review-log](./zenify_review-log) | Xem learning-capture log của znf:review (summary local; --json cho M6) |
| [zenify rules](./zenify_rules) | quản lý và kiểm rule team (F1/F2/F3) |
| [zenify rules lint](./zenify_rules_lint) | chặn tiếng Việt trong file agent-read (skill .md, Go, rules) |
| [zenify secret-scan](./zenify_secret-scan) | Quét secret trong cây thư mục (dùng cho CI + kiểm tra tay) |
| [zenify skills](./zenify_skills) | quản lý plugin skill znf |
| [zenify skills install](./zenify_skills_install) | materialize coding skill (leg-1) cho repo hiện tại vào .claude/skills |
| [zenify skills sync](./zenify_skills_sync) | materialize plugin znf vào ~/.claude/skills/znf |
| [zenify spec](./zenify_spec) | soi vòng đời spec (planned/built) và registry contract từ store spec |
| [zenify spec contracts](./zenify_spec_contracts) | registry contract: repo/collection mỗi spec khai qua _Blast-radius:/_DB: |
| [zenify spec status](./zenify_spec_status) | liệt kê trạng thái vòng đời từng spec (planned/in-progress/built/built?/superseded/unknown) |
| [zenify standards](./zenify_standards) | Kiểm test-traceability — mỗi FR có một test thật trên đĩa (advisory, fail-open) |
| [zenify up](./zenify_up) | Onboard workspace: wizard tương tác trong terminal, còn không thì in kế hoạch dry-run (--apply để chạy headless) |
| [zenify update](./zenify_update) | Nâng zenify lên bản release mới nhất (brew / scoop / install script) |
| [zenify version](./zenify_version) | In version của zenify |
| [zenify visual](./zenify_visual) | Visual-regression golden-diff (Playwright chạy trong Docker, đã ghim phiên bản) |
| [zenify visual check](./zenify_visual_check) | So từng route với baseline; --update để chụp lại baseline |
| [zenify wt](./zenify_wt) | Quản lý git worktree + môi trường dev: tạo, liệt kê, gỡ, dọn worktree theo slug |
| [zenify wt config](./zenify_wt_config) | Hiện worktree.json đã resolve (hoặc --port <key> để xem port đã cấp) |
| [zenify wt ls](./zenify_wt_ls) | Liệt kê worktree trong repo này (git ⋈ state), kèm trạng thái running/merged |
| [zenify wt new](./zenify_wt_new) | Tạo worktree: branch + port + env đã seed + deps |
| [zenify wt path](./zenify_wt_path) | In đường dẫn tuyệt đối của worktree theo slug |
| [zenify wt promote](./zenify_wt_promote) | Chuyển node_modules symlink của worktree thành bản copy CoW riêng |
| [zenify wt rm](./zenify_wt_rm) | Gỡ một worktree (từ chối worktree dirty/detached/chưa merge nếu không có --force) |
| [zenify wt sweep](./zenify_wt_sweep) | Dọn mọi worktree đã merge và sạch trong repo này (hoặc cả workspace với --all) |
| [zenify wt url](./zenify_wt_url) | In http://localhost:<port> của một slug |
| [zenify wt wire](./zenify_wt_wire) | Trỏ file env của worktree này sang các peer service đang được sửa |

## Lệnh nội bộ (hook)

Không có trang riêng; hook và skill gọi chúng, người dùng không gõ tay.

| Lệnh | Mô tả |
|---|---|
| `zenify git-guard` | Hook PreToolUse: chặn commit/merge/push vào deploy branch + secret đã stage |
| `zenify hooks-run` | Bộ điều phối nội bộ cho hook znf (chỉ chạy trong workspace, fail-open) |
| `zenify observe count` | PreToolUse hook: đếm dispatch Task + soft-cap warn (không chặn) |
| `zenify observe meter` | PostToolUse hook: đo lượng tool-output per-session (passive, không sửa output) |
| `zenify review-advise-gate` | Cơ học quyết định có gọi adviser LLM không ở POST của znf:review (AdviseInput JSON qua stdin, seam POST) |
| `zenify review-bundle` | Cơ học chia diff lớn thành bundle cụm-file cho znf:review (seam BUNDLE) |
| `zenify review-doctrine` | Cơ học strip dòng chỉ-verdict khỏi ## Verified của ship-pack (text qua stdin, seam doctrine) |
| `zenify review-log record` | Ghi một review record vào store local (Record JSON qua stdin, seam POST) |
| `zenify review-verify` | Cơ học verify findings của znf:review vs file thật (findings JSON qua stdin, seam VERIFY) |
