---
title: Lệnh CLI
---

# Lệnh CLI

Dùng phần này khi bạn đã biết mình cần kết quả gì và muốn tìm đúng lệnh `zenify`.

Mỗi lệnh có một trang riêng với cú pháp, cờ và ví dụ. Nội dung cú pháp và cờ lấy thẳng từ binary, nên luôn khớp với phiên bản bạn đang chạy. Kiểm tra phiên bản bằng `zenify version`.

## Tìm lệnh theo việc cần làm

| Việc cần làm | Bắt đầu với | Lệnh thường dùng |
|---|---|---|
| Cài kit vào workspace, kiểm tra môi trường | [zenify up](./zenify_up) | [zenify doctor](./zenify_doctor), [zenify update](./zenify_update), [zenify version](./zenify_version), [zenify down](./zenify_down) |
| Mở worktree cho một task, chạy dev server | [zenify wt new](./zenify_wt_new) | [zenify wt ls](./zenify_wt_ls), [zenify wt url](./zenify_wt_url), [zenify wt wire](./zenify_wt_wire), [zenify wt rm](./zenify_wt_rm), [zenify wt sweep](./zenify_wt_sweep) |
| Sửa lỗi trên production | [zenify hotfix baseref](./zenify_hotfix_baseref) | [zenify wt new](./zenify_wt_new) với `--type hotfix` |
| Đọc dữ liệu thật trước khi viết code | [zenify db-read](./zenify_db-read) | [zenify db-perf](./zenify_db-perf) |
| Kiểm tra thay đổi chạm tài nguyên chung | [zenify gate participants](./zenify_gate_participants) | [zenify spec contracts](./zenify_spec_contracts) |
| Kiểm spec và plan trước khi code | [zenify analyze](./zenify_analyze) | [zenify standards](./zenify_standards), [zenify spec status](./zenify_spec_status) |
| Kiểm thử UI và E2E | [zenify visual check](./zenify_visual_check) | [zenify e2e lint](./zenify_e2e_lint), [zenify e2e run](./zenify_e2e_run) |
| Bảo vệ branch deploy, quét secret, kiểm rule | [zenify guard install](./zenify_guard_install) | [zenify secret-scan](./zenify_secret-scan), [zenify rules lint](./zenify_rules_lint) |
| Release và ghi nhận rủi ro | [zenify release-report](./zenify_release-report) | [zenify release-note](./zenify_release-note), [zenify review-log](./zenify_review-log) |
| Knowledge store và tài liệu | [zenify docs sync](./zenify_docs_sync) | [zenify docs gen](./zenify_docs_gen), [zenify config](./zenify_config) |
| Quản lý plugin skill | [zenify skills sync](./zenify_skills_sync) | [zenify skills install](./zenify_skills_install) |
| Theo dõi phiên Claude Code | [zenify observe report](./zenify_observe_report) | [zenify observe statusline install](./zenify_observe_statusline_install) |
| Gom repo về layout chuẩn | [zenify migrate](./zenify_migrate) | |

## Tất cả lệnh

| Lệnh | Mô tả |
|---|---|
| [zenify analyze](./zenify_analyze) | Kiểm tra cơ học một cặp spec và plan trước khi code: coverage FR, marker còn sót, cấu trúc Brief. |
| [zenify config](./zenify_config) | Kéo cấu hình chung của team từ knowledge store vào workspace theo một chiều. |
| [zenify cost](./zenify_cost) | Cho biết token Claude Code đi đâu: context mỗi turn, phần subagent, model, skill và dấu hiệu lãng phí, đọc từ transcript local. |
| [zenify db-perf](./zenify_db-perf) | Quét tĩnh một diff để tìm anti-pattern query DB, chia finding thành BLOCKING và ADVISORY. |
| [zenify db-read](./zenify_db-read) | Đọc database dùng chung ở chế độ chỉ đọc, không bao giờ ghi và không in secret. |
| [zenify docs](./zenify_docs) | Nhóm lệnh cho docs layer: đồng bộ knowledge store và sinh trang tham chiếu. |
| [zenify docs gen](./zenify_docs_gen) | Sinh các trang tham chiếu của website này từ binary, hoặc kiểm tra trang đã sinh còn khớp. |
| [zenify docs sync](./zenify_docs_sync) | Đồng bộ knowledge store với remote qua git và dựng lại view docs/ trong workspace. |
| [zenify doctor](./zenify_doctor) | Kiểm tra sức khỏe môi trường ZenifyKit trên máy và báo từng mục đạt hay hỏng. |
| [zenify down](./zenify_down) | Gỡ phần ZenifyKit đã cài vào máy và repo, giữ nguyên code, workspace và knowledge store. |
| [zenify e2e](./zenify_e2e) | Nhóm lệnh cho journey E2E: lint chống test hời và chạy journey Playwright thật trong Docker. |
| [zenify e2e lint](./zenify_e2e_lint) | Kiểm tra cơ học các journey trong `.znf/e2e`, chặn journey thiếu re-fetch, assert hoặc cleanup. |
| [zenify e2e run](./zenify_e2e_run) | Chạy journey E2E trong Docker, trỏ vào dev server đang mở trên máy bạn. |
| [zenify gate](./zenify_gate) | Nhóm lệnh hỗ trợ contract gate, bước quét cross-repo khi bạn chạm tài nguyên chung. |
| [zenify gate participants](./zenify_gate_participants) | Liệt kê các repo tham gia contract gate cùng cách mỗi repo truy cập DB chung. |
| [zenify guard](./zenify_guard) | Nhóm lệnh cho git-guard, hook chặn commit, push và merge vào branch deploy. |
| [zenify guard install](./zenify_guard_install) | Gắn hook git-guard vào ~/.claude/settings.json để chặn thao tác git vào branch deploy. |
| [zenify hotfix](./zenify_hotfix) | Nhóm lệnh hỗ trợ hotfix, hiện có một lệnh con in base ref đã resolve. |
| [zenify hotfix baseref](./zenify_hotfix_baseref) | In base ref mà hotfix của một repo sẽ xuất phát, theo chiến lược khai trong `worktree.json`. |
| [zenify migrate](./zenify_migrate) | Gom các repo đang nằm rải ở gốc workspace vào một thư mục con theo layout chuẩn của team. |
| [zenify observe](./zenify_observe) | Nhóm lệnh theo dõi phiên Claude Code: số lần dispatch subagent và lượng tool output. |
| [zenify observe report](./zenify_observe_report) | Tóm tắt số lần dispatch subagent và lượng tool output của từng phiên Claude Code. |
| [zenify observe statusline](./zenify_observe_statusline) | Vẽ một dòng statusline cho Claude Code với model, ngữ cảnh, số dispatch, tool output và chi phí. |
| [zenify observe statusline install](./zenify_observe_statusline_install) | Ghi key `statusLine` trỏ tới `zenify observe statusline` vào `~/.claude/settings.json`. |
| [zenify release-note](./zenify_release-note) | Ghi một commit ghi chú release mang metadata rủi ro, để release report đọc sau. |
| [zenify release-report](./zenify_release-report) | Sinh báo cáo rủi ro cho một release từ lịch sử git của mọi repo trong workspace. |
| [zenify review-log](./zenify_review-log) | Xem tổng hợp các lần review của `/znf:review` đã ghi lại trên máy bạn. |
| [zenify rules](./zenify_rules) | Nhóm lệnh kiểm tra rule team, hiện có lệnh lint ngôn ngữ cho file agent đọc. |
| [zenify rules lint](./zenify_rules_lint) | Kiểm file mà agent đọc (skill, rule, mã nguồn kit): chặn tiếng Việt, và bắt frontmatter khai sai key `globs:` thay cho `paths:`. |
| [zenify secret-scan](./zenify_secret-scan) | Quét một cây thư mục để tìm secret bị lộ, dùng trong CI và kiểm tra tay trước khi push. |
| [zenify skills](./zenify_skills) | Nhóm lệnh quản lý plugin skill znf và bộ coding skill theo repo. |
| [zenify skills install](./zenify_skills_install) | Gỡ bản coding skill cũ khỏi .claude/skills của repo — chúng đã nằm trong plugin znf. |
| [zenify skills sync](./zenify_skills_sync) | Ghi bộ skill znf từ binary vào ~/.claude/skills/znf và gắn hook znf vào settings. |
| [zenify spec](./zenify_spec) | Nhóm lệnh soi vòng đời spec và registry contract, đọc từ knowledge store. |
| [zenify spec contracts](./zenify_spec_contracts) | Liệt kê repo và collection mà từng spec khai qua hai tag `_Blast-radius:` và `_DB:`. |
| [zenify spec status](./zenify_spec_status) | Liệt kê trạng thái vòng đời của từng spec trong knowledge store. |
| [zenify standards](./zenify_standards) | Kiểm tra mỗi FR trong spec có một test thật trên đĩa mà plan đã khai. |
| [zenify ui-verify](./zenify_ui-verify) | Nhóm lệnh ghi/kiểm tra bằng chứng UI-verify cho gate cơ học của /znf:ship. |
| [zenify ui-verify check](./zenify_ui-verify_check) | Gate fail-closed kiểm tra artifact UI-verify còn khớp fingerprint hiện tại. |
| [zenify ui-verify record](./zenify_ui-verify_record) | Ghi lại artifact UI-verify (screenshot + 3 số đo) cho fingerprint hiện tại. |
| [zenify up](./zenify_up) | Onboard máy vào workspace: clone repo, ghi cấu hình, gắn hook và skill, chuẩn bị knowledge store. |
| [zenify update](./zenify_update) | Nâng cấp binary ZenifyKit lên bản mới nhất bằng đúng kênh đã cài, hoặc chỉ kiểm tra có bản mới. |
| [zenify version](./zenify_version) | In phiên bản binary ZenifyKit đang chạy. |
| [zenify visual](./zenify_visual) | Nhóm lệnh visual regression: chụp từng route và so với ảnh baseline đã commit. |
| [zenify visual check](./zenify_visual_check) | So ảnh chụp từng route với baseline, hoặc chụp lại baseline khi giao diện đổi có chủ ý. |
| [zenify wt](./zenify_wt) | Nhóm lệnh quản lý worktree theo slug: tạo, liệt kê, gỡ, dọn, kèm port và môi trường dev riêng cho từng task. |
| [zenify wt config](./zenify_wt_config) | In cấu hình worktree đã resolve của repo hiện tại, hoặc port sẽ cấp cho một key. |
| [zenify wt ls](./zenify_wt_ls) | Liệt kê worktree trong repo hiện tại, kèm branch, port, trạng thái merge và dev server đang chạy. |
| [zenify wt new](./zenify_wt_new) | Tạo worktree mới cho một task, kèm branch, port riêng và môi trường dev đã seed. |
| [zenify wt path](./zenify_wt_path) | In đường dẫn tuyệt đối của worktree theo slug. |
| [zenify wt promote](./zenify_wt_promote) | Đổi thư mục node_modules dạng symlink của một worktree thành bản copy riêng. |
| [zenify wt rm](./zenify_wt_rm) | Gỡ một worktree đã xong việc, từ chối khi còn thay đổi chưa commit hoặc branch chưa merge. |
| [zenify wt sweep](./zenify_wt_sweep) | Dọn mọi worktree đã merge và sạch trong repo, dừng dev server của chúng trước khi gỡ. |
| [zenify wt url](./zenify_wt_url) | In địa chỉ http://localhost:\<port\> của dev server thuộc một slug. |
| [zenify wt wire](./zenify_wt_wire) | Trỏ file env của worktree hiện tại sang dev server của các service liên quan đang được sửa cùng slug. |

## Lệnh nội bộ

Hook và skill gọi các lệnh này; bạn không cần gõ tay nên chúng không có trang riêng.

| Lệnh | Mô tả |
|---|---|
| `zenify git-guard` | Hook PreToolUse: chặn commit/merge/push vào deploy branch + secret đã stage |
| `zenify hooks-run` | Bộ điều phối nội bộ cho hook znf (chỉ chạy trong workspace, fail-open) |
| `zenify observe count` | PreToolUse hook: đếm dispatch Task + soft-cap warn (không chặn) |
| `zenify observe meter` | PostToolUse hook: đo tool-output per-session; nhắc khi một result \> 50KB hoặc phiên \> 2MB (không sửa output) |
| `zenify review-advise-gate` | Cơ học quyết định có gọi adviser LLM không ở POST của znf:review (AdviseInput JSON qua stdin, seam POST) |
| `zenify review-bundle` | Cơ học chia diff lớn thành bundle cụm-file cho znf:review (seam BUNDLE) |
| `zenify review-doctrine` | Cơ học strip dòng chỉ-verdict khỏi ## Verified của ship-pack (text qua stdin, seam doctrine) |
| `zenify review-log record` | Ghi một review record vào store local (Record JSON qua stdin, seam POST) |
| `zenify review-verify` | Cơ học verify findings của znf:review vs file thật (findings JSON qua stdin, seam VERIFY) |

<!-- Generated by zenify docs gen: intro from internal/docsgen/catalog/cli/index.md, table from the command tree. -->
