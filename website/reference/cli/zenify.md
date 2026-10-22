---
title: zenify
---

[Lệnh CLI](/reference/cli/) / `zenify`

# `zenify`

Binary dòng lệnh của ZenifyKit, chứa mọi lệnh workspace và plugin skill đi kèm.

## Cú pháp

```text
zenify
```

## Tổng quan

`zenify` là binary duy nhất bạn cài. Nó mang các lệnh quản lý workspace, worktree, gate và tài liệu, đồng thời chứa plugin skill `znf:*` cho Claude Code. Cài đặt và bước đầu xem [Cài đặt](/getting-started/install) và [Bắt đầu nhanh](/getting-started/quickstart).

Gõ `zenify <lệnh> --help` để xem cú pháp của từng lệnh. Lệnh có ghi "dry-run mặc định" chỉ in kế hoạch cho tới khi bạn thêm `--apply`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--no-color` | disable colored output |

## Lệnh liên quan

- [zenify analyze](./zenify_analyze): Kiểm tra cơ học một cặp spec và plan trước khi code: coverage FR, marker còn sót, cấu trúc Brief.
- [zenify await](./zenify_await): Dừng một lượt chạy không người ở cổng cần người duyệt, ghi bản ghi await rồi thoát mã 8.
- [zenify broker](./zenify_broker): Nhóm lệnh làm việc với credential broker của zenify.
- [zenify catalog](./zenify_catalog): In danh mục dịch vụ giám sát (datasource Grafana, tên log, trace, metric) dưới dạng YAML.
- [zenify config](./zenify_config): Kéo cấu hình chung của team từ knowledge store vào workspace theo một chiều.
- [zenify cost](./zenify_cost): Cho biết token Claude Code đi đâu: context mỗi turn, phần subagent, model, skill và dấu hiệu lãng phí, đọc từ transcript local.
- [zenify data](./zenify_data): Nhóm lệnh chạy script dữ liệu với credential DB do broker cấp, không để lộ giá trị trong output.
- [zenify db-perf](./zenify_db-perf): Quét tĩnh một diff để tìm anti-pattern query DB, chia finding thành BLOCKING và ADVISORY.
- [zenify db-read](./zenify_db-read): Đọc database dùng chung ở chế độ chỉ đọc, không bao giờ ghi và không in secret.
- [zenify docs](./zenify_docs): Nhóm lệnh cho docs layer: đồng bộ knowledge store và sinh trang tham chiếu.
- [zenify doctor](./zenify_doctor): Kiểm tra sức khỏe môi trường ZenifyKit trên máy và báo từng mục đạt hay hỏng.
- [zenify down](./zenify_down): Gỡ phần ZenifyKit đã cài vào máy và repo, giữ nguyên code, workspace và knowledge store.
- [zenify e2e](./zenify_e2e): Nhóm lệnh cho journey E2E: lint chống test hời và chạy journey Playwright thật trong Docker.
- [zenify env](./zenify_env): Quản lý biến môi trường cá nhân cho tool dev và agent trong `<workspace>/.zenify/.env`, không phải env chạy app.
- [zenify gate](./zenify_gate): Nhóm lệnh hỗ trợ contract gate, bước quét cross-repo khi bạn chạm tài nguyên chung.
- [zenify guard](./zenify_guard): Nhóm lệnh cho git-guard, hook chặn commit, push và merge vào branch deploy.
- [zenify hotfix](./zenify_hotfix): Nhóm lệnh hỗ trợ hotfix, hiện có một lệnh con in base ref đã resolve.
- [zenify migrate](./zenify_migrate): Gom các repo đang nằm rải ở gốc workspace vào một thư mục con theo layout chuẩn của team.
- [zenify npm](./zenify_npm): Nhóm lệnh cấu hình npm cho package private trên GitHub Packages.
- [zenify obs](./zenify_obs): Tóm tắt sức khỏe một route hoặc một trace qua broker, đã che PII và giới hạn kích thước.
- [zenify observe](./zenify_observe): Nhóm lệnh theo dõi phiên Claude Code: số lần dispatch subagent và lượng tool output.
- [zenify pm](./zenify_pm): Chọn đúng package manager JS của project (pnpm, yarn, npm, bun) rồi chạy nó, để không sinh thêm lockfile thứ hai.
- [zenify redis-read](./zenify_redis-read): Chạy một lệnh Redis chỉ đọc qua credential broker.
- [zenify release-note](./zenify_release-note): Ghi một commit ghi chú release mang metadata rủi ro, để release report đọc sau.
- [zenify release-report](./zenify_release-report): Sinh báo cáo rủi ro cho một release từ lịch sử git của mọi repo trong workspace.
- [zenify reviewer](./zenify_reviewer): In ra engine review mà kit sẽ dùng ở phiên này, agent hoặc codex, kèm lý do khi rơi về agent.
- [zenify route-log](./zenify_route-log): Xem các record routing model do skill ghi (số liệu plan từ znf:analyze), đếm theo site, model và gate.
- [zenify rules](./zenify_rules): Nhóm lệnh kiểm tra rule team, hiện có lệnh lint ngôn ngữ cho file agent đọc.
- [zenify secret-scan](./zenify_secret-scan): Quét một cây thư mục để tìm secret bị lộ, dùng trong CI và kiểm tra tay trước khi push.
- [zenify skills](./zenify_skills): Nhóm lệnh quản lý plugin skill znf và bộ coding skill theo repo.
- [zenify spec](./zenify_spec): Nhóm lệnh soi vòng đời spec và registry contract, đọc từ knowledge store.
- [zenify standards](./zenify_standards): Kiểm tra mỗi FR trong spec có một test thật trên đĩa mà plan đã khai.
- [zenify ui-verify](./zenify_ui-verify): Nhóm lệnh ghi/kiểm tra bằng chứng UI-verify cho gate cơ học của /znf:ship.
- [zenify up](./zenify_up): Onboard máy vào workspace: clone repo, ghi cấu hình, gắn hook và skill, chuẩn bị knowledge store.
- [zenify update](./zenify_update): Nâng cấp binary ZenifyKit lên bản mới nhất bằng đúng kênh đã cài, hoặc chỉ kiểm tra có bản mới.
- [zenify version](./zenify_version): In phiên bản binary ZenifyKit đang chạy.
- [zenify visual](./zenify_visual): Nhóm lệnh visual regression: chụp từng route và so với ảnh baseline đã commit.
- [zenify wt](./zenify_wt): Nhóm lệnh quản lý worktree theo slug: tạo, liệt kê, gỡ, dọn, kèm port và môi trường dev riêng cho từng task.

<!-- Generated by zenify docs gen: prose from internal/docsgen/catalog, facts from the command tree. -->
