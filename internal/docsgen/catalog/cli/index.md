---
summary: Dùng phần này khi bạn đã biết mình cần kết quả gì và muốn tìm đúng lệnh `zenify`.
---
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
