---
title: Changelog
---

# Changelog

Mỗi release một mục, viết cho người dùng kit, không phải danh sách commit. Mục của một bản được viết **trước khi tag**, trong PR docs của lần cắt release, theo [Cắt release cho zenify-kit](/guides/release#cắt-release-cho-zenify-kit). Workflow release từ chối tag chưa có mục ở đây, và dùng chính mục đó làm nội dung GitHub Release, nên trang này không thể cũ hơn bản đang phát hành.

Bản trước `v0.22.0`: xem [GitHub Releases](https://github.com/ZenifyAIContactCenter/zenify-kit/releases).

## v0.53.1

*2026-10-29*

- **Pane nonstop gọn hơn:** tab Need Human bỏ dòng tiêu đề `CẦN ANH` lặp lại tên tab; câu hỏi chờ bạn hiện ngay dưới hàng tab, mục `CHỜ MERGE` giữ nguyên.

## v0.53.0

*2026-10-29*

- **Pane nonstop cạnh chat:** gõ `/znf:nonstop` (hoặc `/nonstop-pane`) mở một pane `Nonstop` bên phải chat, tự cập nhật, không tốn lượt chat. Hàng tab `Need Human │ In Progress │ Pending │ Done` (tab đang chọn tô màu), khu `Thống kê` cố định ở đáy và dòng `Xem thêm thông tin` mở trang định nghĩa chỉ số trong knowledge view của workspace. Task và câu hỏi có priority `P0`–`P3`, bấm id để mở rộng, PR và issue là link. Session thường không tự mở pane. Pane chỉ dock cạnh chat khi Claude Code chạy fullscreen. Xem [Pane nonstop](/guides/nonstop-pane).
- **Theo dõi PR và merge từ pane:** khu `ĐANG WATCH` hiện CI, `Zenie Review` và số review thread mở của mọi PR còn mở. PR chỉ vào `CHỜ MERGE` khi CLEAN hoặc UNSTABLE, Zenie pass và 0 thread mở. Nút `[merge]` cần bấm hai lần trong 5 giây; ngay trước khi merge pane đọc lại PR và chạy `gh pr merge --merge --match-head-commit <sha>`, nên head hay base đổi thì không merge. `[c]` copy lệnh merge.
- **Status line nonstop:** `nonstop ▶ · In Progress: 1/2 · Need Human: 5 · Pending: 2 · Done: 5/8 · Ready: 2 · CI ✗: 1 · Threads: 3 · 11 pts · hit 71%`; mục bằng 0 thì ẩn.
- **Lệnh mới `zenify nonstop stats`:** nguồn duy nhất cho thang size và thống kê của nonstop (`--json` cho pane và orchestrator, `--dir` chọn thư mục ledger). Thang S/M/L tự học từ thời gian làm thật (phân vị 25/50/90 của 60 task gần nhất, cần ít nhất 20 task; trước đó dùng 5/20/60 phút), kèm hit rate, điểm/ngày, tỉ lệ thời gian review, tier T1/T2/T3 và gợi ý calibration theo repo và loại task. Xem [`zenify nonstop stats`](/reference/cli/zenify_nonstop_stats).
- **Tự đo thời gian:** pane ghi `durations.log` cạnh ledger cho mỗi lượt worker và reviewer chạy xong, tách `work` với `review`; thời gian chờ người không bị tính. Dòng `estimates.md` lên v2 13 trường (thêm `work`, `review`, `path` của brainstorming, `tier` của review); dòng v1 vẫn đọc được.
- **Skill nonstop:** ledger chỉ ghi dòng 10 trường có `priority` (không bảng, không `###`), re-entry gặp section dạng bảng thì viết lại trước khi dispatch; orchestrator không còn ghi thời gian bằng tay mà lấy thang và thống kê từ `zenify nonstop stats --json --dir <ledger>`.

## v0.52.0

*2026-10-29*

- **Mã ticket trong tiêu đề PR:** branch có `zen-<n>` ở ranh giới slug (ví dụ `<user>/fix/zen-123-session-timeout`) thì `/znf:ship` đặt `ZEN-<n>` ngay sau dấu hai chấm của tiêu đề: `fix(session): ZEN-123 <mô tả>`. Branch không có mã thì tiêu đề giữ nguyên. Với stacked PR, thân PR của mọi lớp trừ lớp trên cùng có dòng `Part of ZEN-<n>`. Xem [Mã ticket trong tiêu đề PR](/workflows/ship#mã-ticket-trong-tiêu-đề-pr).
- **Slug mang mã ticket:** `zenify wt new zen-123-session-timeout --type fix` tạo branch `<user>/fix/zen-123-session-timeout`, đúng chỗ ship đọc mã. Xem [Worktree theo slug](/concepts/worktree-per-slug).
- **`zenify release-report` chạy theo tag:** team đã bỏ nhánh `release<N>`, nên lệnh giờ nhận tag `vYYMMDD.n` (`zenify release-report v261104.0`) và ghi `<tag>.md`. Mỗi repo được tính từ tag liền trước tới tag đó, kể cả tag hotfix, nên tuần có hotfix ra nhiều báo cáo. Không truyền tag thì lệnh lấy tag mới nhất trong cả workspace và chỉ báo cáo các repo mang tag đó. Repo được ghim trong `.zenify/release-repos.txt` mà chưa có tag nào có dòng "chưa có tag"; ở chế độ tự dò, repo không có tag bị bỏ khỏi báo cáo. Bỏ ngày cắt nhánh và dòng "Không ship" khỏi báo cáo. Xem [Release và báo cáo](/guides/release).

## v0.51.1

*2026-10-29*

- **Base hotfix lấy từ một chỗ duy nhất:** `zenify wt new --type hotfix` và `zenify hotfix baseref` giờ luôn ra cùng một ref. Có `hotfixBaseRef` trong `.claude/worktree.json` thì dùng nó, không có thì dùng `baseRef` của repo kèm một dòng cảnh báo. Trước đây hai lệnh có thể ra hai nhánh khác nhau. Xem [Base ref, hotfix base, port block](/concepts/base-ref-and-ports).
- **Bỏ chiến lược `release-latest`:** chiến lược này đoán nhánh `release<N>` lớn nhất, nên khi team thôi cắt nhánh release thì hotfix có thể tách từ một bản đã cũ nhiều tuần. Giờ lệnh báo lỗi và hướng dẫn khai `"hotfix": {"baseStrategy": "custom"}` cùng `"hotfixBaseRef"`, ví dụ `origin/main`. Các repo zenify đã khai sẵn `origin/main`.

## v0.51.0

*2026-10-27*

- Lệnh mới `zenify ensure --workspace <dir>`: ghi hook, model phiên chính và `CLAUDE_CODE_SUBAGENT_MODEL` giống `zenify up`, dùng cho máy không chạy `zenify up`, ví dụ worker zenie trong container. Worker nhờ đó chạy cùng cấu hình với máy local trên cùng bản kit.

## v0.50.0

*2026-10-27*

- **`zenify feature` bật/tắt tính năng tuỳ chọn:** `zenify feature github.issues on|off` ghi khoá `features:` trong `.zenify-overlay.yaml`, `zenify feature` không đối số liệt kê trạng thái. Đầu phiên có dòng `features: …` khi có tính năng đang bật. Khi `github.issues` bật, ship gắn một issue GitHub trước khi mở PR (dùng issue có sẵn trong phiên, spec hay ledger, kiểm trùng trước khi tạo mới). Xem [Gắn issue GitHub](/workflows/ship#gắn-issue-github-tuỳ-chọn).
- **`ci-watch` chờ PR sẵn sàng merge:** CI xanh chưa đủ, skill chờ PR ở trạng thái CLEAN rồi in URL PR kèm lệnh `! gh pr merge …` cho bạn tự chạy; bị chặn thì liệt kê review thread chưa xử lý. Skill không tự merge. Xem [Sau khi mở PR: `--watch`](/workflows/ship#sau-khi-mở-pr-watch).
- **Skill mới `znf:nonstop`:** chạy cả ngày làm việc từ một lệnh: lập plan đầu ngày chờ bạn duyệt, giao việc liên tục cho các worker, review từng việc trước khi mở PR, tổng kết cuối ngày kèm việc chuyển sang hôm sau. Chỉ lấy issue và PR của chính bạn hoặc được giao cho bạn; không bao giờ merge. Xem [Chọn workflow](/workflows/).
- **Review không bỏ sót file mới chưa commit:** khi review một cây chưa commit, file mới chưa track được đếm dòng và đưa vào bundle (file nhị phân tính 0 dòng, tên file Unicode giữ nguyên), nên diff lớn không còn lọt file mới khỏi review.
- **QA tách tiêu chí nhiều điều kiện:** một dòng tiêu chí dạng "khi A thì X; khi B thì Y" được `znf:qa` viết thành một cặp `Bước:`/`Thấy:` cho mỗi điều kiện, và journey có một test cho mỗi cặp, nên bản sửa chỉ xử lý một điều kiện sẽ bị đỏ. Xem [Journey E2E](/workflows/ui-testing#journey-e2e).

## v0.49.0

*2026-10-26*

- **`zenify e2e login` và `zenify e2e run --local` đọc tài khoản theo vai từ file:** khi `ZNF_E2E_ACCOUNTS_FILE` trỏ tới file JSON dạng `{"<vai>": {"domain", "email", "password"}}`, lệnh lấy tài khoản của `--role` từ file đó, không gọi broker và không cần manifest khai `broker:`. Không có `--role` thì dùng vai `administrator`. Vai không có trong file, file không đọc được, JSON hỏng hay trường rỗng đều thoát mã 2, thông báo không bao giờ in giá trị. `e2e run` chạy Docker và `visual check` không đọc biến này. Dùng cho sandbox của worker, nơi tài khoản broker trên staging không đăng nhập được. Xem [Journey E2E](/workflows/ui-testing#journey-e2e).
- **Trang hướng dẫn Zenie review trên PR:** giải thích khi nào Zenie review, cách chấm điểm, check `Zenie Review` xanh hay đỏ khi nào, và các lệnh `@zenieai` gõ được trên PR. Xem [Zenie review trên PR](/guides/zenie-review).

## v0.48.0

*2026-10-25*

- **`zenify e2e run --local` chạy journey không cần Docker:** journey chạy trên Playwright của máy, dùng chung harness với `zenify e2e login`, chỉ nhận app loopback (`--port` hoặc `--base-url` localhost). Lệnh giữ khóa `harness.lock` suốt lần chạy, nên hai lần `--local` trên cùng máy đi lần lượt. Đã có đúng revision `chromium` và `chromium-headless-shell` (cài trọn vẹn) thì không cần mạng. `--prepare` chỉ chuẩn bị harness rồi thoát. Xem [Journey E2E](/workflows/ui-testing#journey-e2e).
- **Journey gắn kịch bản nghiệm thu đã chốt:** journey có dòng `// @scenario ZEN-<n>` được lint với `.znf/e2e/scenarios/ZEN-<n>.md`. Mỗi test mang `@ac:<n>` trong tiêu đề, mọi `@ac` loại `e2e` phải có test, mọi chuỗi trong câu lệnh `expect` phải có nguyên văn trong kịch bản, và mọi kiểu skip (`test.skip`, `test.step.skip`, `.skip()` của `testInfo`…) bị chặn. Journey không có dòng `@scenario` giữ luật cũ. Xem [Journey E2E](/workflows/ui-testing#journey-e2e).
- **Skill `/znf:qa` cho run QA của worker:** viết kịch bản nghiệm thu đóng băng trước khi có code dev, journey phải đỏ ở một `expect` trên code gốc, và chế độ `bind` gắn journey vào code đã xong mà không sửa expect. Xem [/znf:qa](/reference/skills/qa).
- **Chế độ không người chạy ui-verifier khi sandbox có web app:** khi biến `ZNF_SANDBOX_WEB_PORT` được đặt, agent đăng nhập bằng `zenify e2e login --port $ZNF_SANDBOX_WEB_PORT --role <vai>`, dispatch ui-verifier như chế độ người, và chạy journey bằng `zenify e2e run --local`. Không có biến đó thì hành vi cũ giữ nguyên. Xem [Diff đụng UI](/guides/autonomous#ui).
- **Resume một câu hỏi bằng `answered:`:** worker nhận thêm prompt `resume <id> answered: <câu trả lời>` cho bản ghi `needs-human` là câu hỏi, bên cạnh `approved` và `rejected: <lý do>`. Xem [Resume](/guides/autonomous#resume).

## v0.47.0

*2026-10-25*

- **`znf:ui-verifier` điều khiển trình duyệt qua `zenify browser` thay cho Playwright MCP:** lệnh mới `zenify browser -s=<phiên> <lệnh>` chạy `@playwright/cli` 0.1.22 đã ghim. Mỗi phiên có thư mục riêng dưới `~/.zenify/playwright/sessions/<phiên>/` và in đường dẫn tuyệt đối cho mọi file nó ghi, nên nhiều verifier chạy song song được mà không lẫn trang của nhau. Lệnh chỉ nhận các lệnh con verifier cần (`open`, `goto`, `snapshot`, `find`, `click`, `fill`, `eval`, `screenshot`, `console`, `resize`, `state-load`, `close`…), và từ chối `run-code`, `upload`, `close-all`, cờ `--config`/`--profile`/`--persistent` cùng `--filename` có đường dẫn. Xem [Kiểm UI](/workflows/ui-testing) và [`zenify browser`](/reference/cli/zenify_browser).
- **`zenify up` cài playwright-cli và gỡ Playwright MCP của kit:** workspace có repo frontend được cài CLI cùng chromium 1247. Đăng ký MCP `playwright` do kit tạo bị gỡ khỏi Claude và Codex, cùng các quyền `mcp__playwright__*`; đăng ký bạn tự cấu hình được giữ nguyên. `zenify doctor` giờ kiểm CLI và chromium thay cho MCP. Sau khi nâng cấp phải chạy `zenify up --apply` một lần, nếu không ui-verifier báo BLOCKED vì thiếu playwright-cli.

## v0.46.0

*2026-10-24*

- **`zenify e2e run --base-url <origin>` chạy journey trên staging:** thay `--port` bằng `--base-url` để journey chạy trên một origin có sẵn thay cho dev server local, ví dụ `https://staging.example.com`. Origin phải nằm trong `baseUrls` của `.znf/e2e/e2e.config.json` (so khớp scheme, host và port; path bị bỏ qua), nếu không lệnh thoát mã 2 trước khi chạy Docker. Container nhận đúng origin đã qua kiểm tra, không nhận path hay query. `--base-url` loại trừ `--port`: phải đưa đúng một trong hai. Xem [Journey E2E](/workflows/ui-testing#journey-e2e).
- **Broker nhận OIDC token của GitHub Actions trên `/token`:** workflow E2E không có token người dùng có thể gửi `{"oidc_token":"..."}` tới `POST /token` để nhận role `e2e`. Broker chỉ cấp role này cho workflow có `workflow_ref` nằm trong khoá `e2e_workflows` của config broker (mặc định rỗng: không nhận OIDC). Role `e2e` chỉ gọi được `POST /e2e/account`, mọi route khác trả 403. Deploy image broker mới trước rồi mới thêm `e2e_workflows`, vì image cũ không khởi động khi gặp khoá lạ. Xem [Role `e2e` cho GitHub Actions](/guides/broker-host#role-e2e).
- **Skill `/znf:top-issues` khảo sát Grafana và xếp top 5 issue hệ thống:** gõ `/znf:top-issues` (mặc định prd, thêm `--stg` cho staging, `--since <duration>` đổi cửa sổ tối đa 48h, liệt kê service id để thu hẹp) để skill đọc alert, lỗi theo route, p95, log lỗi và backlog queue, chấm điểm theo đối tượng, tenant, money path và xu hướng, rồi đề xuất cách xử lý cho từng issue. Skill chỉ đọc; issue bạn chọn được chuyển sang `/znf:fix`. Báo cáo ghi vào `docs/exports/_cross/`. Xem [/znf:top-issues](/reference/skills/top-issues).
- **brokerclient tự dùng OIDC trong GitHub Actions:** khi chạy trong Actions và job được cấp `permissions: id-token: write`, các lệnh gọi broker (như `zenify e2e run`) tự xin OIDC token và đổi lấy token role `e2e`, không cần đăng nhập GitHub. Ngoài Actions hoặc thiếu `id-token: write` thì luồng đăng nhập cũ giữ nguyên.

## v0.45.1

*2026-10-23*

- **Skill `znf:ci-watch` theo dõi CI của commit vừa push:** sau khi push, gõ `/znf:ci-watch` để Claude theo dõi GitHub Actions của commit `HEAD` ở chế độ nền và đánh thức phiên khi CI chạy xong. Skill chỉ báo xanh khi danh sách run đứng yên ít nhất 60 giây. Có run lỗi thì skill tự giao URL run cho `/znf:fix`, rồi theo dõi commit mới; vòng 2 vẫn đỏ thì dừng và báo. Xem [/znf:ci-watch](/reference/skills/ci-watch).
  - `/znf:ship --watch` tự gọi `znf:ci-watch` sau khi mở PR.

## v0.45.0

*2026-10-22*

- **Worker tự động kiểm UI bằng `needs-human`:** khi `ZNF_AUTONOMOUS=1`, `zenify ui-verify check` mà chưa có bằng chứng UI-verify khớp HEAD thì ghi một bản ghi chờ duyệt `needs-human` (summary bắt đầu bằng `cần kiểm UI:`), in `id` rồi thoát mã 8, thay vì mã 1 (BLOCKED). Bước 4 của `/ship` cũng dừng bằng `zenify await needs-human` thay vì dispatch ui-verifier khi diff có file hiển thị UI. Ngoài chế độ tự động, lệnh giữ nguyên hành vi cũ. Xem [Chế độ tự động](/guides/autonomous#ui).
- **`wt` seed deps từ `ZENIFY_DEPS_CACHE`:** khi biến này được đặt và có `<repo>/<thư mục deps>` trong cache, `zenify wt new` (chế độ `symlink` hoặc `clone`) lấy deps từ cache nếu `package-lock.json` của cache khớp từng byte với worktree. Lock khác hoặc không đọc được thì bỏ cache và chạy lệnh install như trước. Không đặt biến thì hành vi không đổi.
- **Skill `run` có quy tắc khởi động cho sandbox của worker:** khi `ZNF_AUTONOMOUS=1`, skill trỏ tới `references/autonomous-sandbox.md`: chỉ khởi động app mà thay đổi cần, chạy `worker-env.mjs` trước, be chạy Node 16, đọc port từ `deploy/worker/AGENTS.md`. Sandbox không có FE nên không có URL cho `znf:ui-verifier`; kiểm bằng request thật và trích output.

## v0.44.0

*2026-10-22*

- **Chế độ tự động cho worker không người trông (`ZNF_AUTONOMOUS=1`):** khi biến này bật, dòng đầu session nhắc agent đọc `_shared/autonomous.md`. Ở các cổng duyệt, agent tự quyết và ghi lại thành một dòng `Ruling`. Riêng spec, plan, ghi dữ liệu production, merge, xoá không khôi phục được, revert hay tắt production và credential thì agent dừng lại chờ người. Xem [Chế độ tự động](/guides/autonomous).
  - Lệnh mới `zenify await <spec|plan|data-write|irreversible|needs-human>` ghi một bản ghi chờ duyệt vào `~/.zenify/await/` rồi thoát mã 8; `zenify await show <id>` in lại bản ghi đó.
- **Role `agent` trên broker:** token cài đặt GitHub App (`ghs_`) của bot có trong `agent_ids` nhận role `agent`. Role này đọc được staging và production, ghi được staging. Nó không nhận credential ghi production, cũng không nhận credential lumi-agent trên production. Danh tính agent lấy từ `GH_TOKEN`. Nếu `ZNF_TASK_ID` được đặt, mỗi dòng audit của broker ghi kèm mã task đó. Xem [Broker cho người dùng](/guides/broker-client) và [Broker host](/guides/broker-host).
  - `zenify data run --prd --write` chạy dưới role `agent` thì dừng lại và in sẵn lệnh `zenify await data-write`, kèm chuỗi lệnh để lead chạy trên máy mình.
- **`zenify pm`:** chạy đúng trình quản lý package của repo (pnpm, yarn, npm hoặc bun), chọn theo trường `packageManager` hoặc theo lockfile. Không xác định được thì lệnh từ chối, không đoán. `zenify up` cài thêm shim `wt` và `pm` vào `~/.local/bin`. Xem [zenify pm](/reference/cli/zenify_pm).
- **`zenify wt base`:** in base ref của worktree hiện tại (`wt.base`, nếu không có thì `baseRef`, cuối cùng là `origin/main`). Bước review của `/ship` và SDD giờ tính merge-base từ base ref này. Xem [zenify wt base](/reference/cli/zenify_wt_base).

## v0.43.0

*2026-10-21*

- **NPM_TOKEN cho package private trên GitHub Packages:** khi một repo trong workspace cài package từ `npm.pkg.github.com` (`.npmrc` đọc token từ `${NPM_TOKEN}`), `zenify up` thêm dòng `export NPM_TOKEN="${NPM_TOKEN:-$(gh auth token 2>/dev/null)}" # zenify-npm-token` vào profile shell và xin thêm scope `read:packages` cho `gh` khi cần. `NPM_TOKEN` bạn đã tự đặt thì được giữ nguyên. Xem [Onboard workspace](/guides/onboard-workspace).
  - Lệnh mới `zenify npm login` làm riêng bước này, kể cả khi không có repo nào cần hoặc khi đứng ngoài workspace. Xem [zenify npm login](/reference/cli/zenify_npm_login).
  - `zenify doctor` có mục `npm token (GitHub Packages)`, và `zenify doctor --fix` tự sửa mục này. Trong terminal, bước sửa có thể mở trình duyệt để cấp scope `read:packages`.
  - Mục này chỉ xét account github.com đang active, và báo `gh auth status timed out` nếu `gh` không trả lời sau 10 giây.
  - Lưu ý: `NPM_TOKEN` là toàn bộ token của `gh auth token`, không chỉ scope `read:packages`. Gỡ bằng cách xoá dòng có `# zenify-npm-token`.
- **`zenify db-read --target lumi`:** đọc MySQL của lumi-agent qua broker, chỉ `sql` và `tables`; trên production chỉ dành cho lead. Xem [Broker cho người dùng](/guides/broker-client). Người vận hành broker cần thêm tunnel staging, xem [Broker host](/guides/broker-host).

## v0.42.0

*2026-10-18*

- **Tóm tắt route và trace:** lệnh mới `zenify obs <id> --route "<METHOD path>" --prd|--stg` cho biết route chậm hay lỗi (tốc độ, p50/p95/p99, tỉ lệ lỗi, trace mẫu), và `zenify obs --trace <trace-id> --prd|--stg` tóm tắt một trace cùng log liên quan. Broker che PII theo mẫu (tên và địa chỉ viết tự do không được che) và cắt output ở 32 KB. Xem [zenify obs](/reference/cli/zenify_obs) và [Quan sát và an toàn](/guides/observe-and-safety#zenify-obs).
  - Agent điều tra lỗi hoặc chậm chạy `zenify obs` trước, chỉ dùng tool Grafana trực tiếp khi cần chi tiết. Mã thoát 5 nghĩa là broker chưa bật `obs`.
  - Công thức "trace sang log" trong tài liệu agent sửa lại: tra app Loki của từng service từ `zenify catalog` thay vì selector khớp mọi giá trị mà Loki từ chối.
- **Broker:** route mới `POST /obs/route` và `POST /obs/trace`, bật cùng khối `catalog:`. Người vận hành broker xem [Danh mục dịch vụ](/guides/broker-host#danh-muc-dich-vu).

## v0.41.0

*2026-10-18*

- **Danh mục dịch vụ:** lệnh mới `zenify catalog [id…]` in tên thật của từng service và datastore trên Loki (`app`, `host`), Tempo (`service.name`), Prometheus (`job`) và PostHog, cho cả prd và stg. Agent tra danh mục trước, chỉ tự dò khi danh mục không có. Nguồn là `catalog/services.yaml` trong repo monitoring, broker đọc từ GitHub và cache 5 phút. Xem [zenify catalog](/reference/cli/zenify_catalog).
  - `zenify catalog drift` so danh mục với label đang có thật trên Grafana và thoát mã 6 khi lệch. Dòng ERROR nghĩa là không đọc được nguồn đó, không phải lệch.
  - Exit code mới: 5 khi broker chưa có danh mục, 6 khi có drift. Id không có trong danh mục thoát mã 2.
- **Broker:** route mới `POST /catalog` và `POST /catalog/drift`, chỉ bật khi cấu hình broker có khối `catalog:` và secret `CATALOG_GITHUB_TOKEN`. Người vận hành broker xem [Danh mục dịch vụ](/guides/broker-host#danh-muc-dich-vu).
- **Agent tự dùng `db-read`, `redis-read`, `data run`:** git-guard bỏ các luật chặn host production và biến `_PRD`, vốn đã lỗi thời từ khi credential production do broker cấp. Agent không còn nhờ bạn chạy `!` để đọc dữ liệu hay chạy script. `zenify up --apply` cho ba lệnh này chạy không hỏi quyền và ngoài sandbox. Xem [Sandbox của Claude Code](/guides/broker-client#sandbox-cua-claude-code).
  - Chặn đọc `.zenify/.env` vẫn giữ và chặt hơn: bung dấu ngoặc nhọn như shell trước khi xét, áp luật lên lệnh bên trong `zenify data run`, chặn `--manifest` trên `db-read`/`redis-read`.
  - `zenify data run` có cả `--prd` và `--write` luôn hiện prompt xác nhận, kể cả khi bật bypass. Mọi lệnh `ssh` cũng luôn hỏi.
  - Khi thiếu key production, `zenify db-read --prd` chỉ sang broker thay vì `zenify env set`.

## v0.40.0

*2026-10-17*

- **Agent tự chạy script dữ liệu:** lệnh mới `zenify data run --stg|--prd [--write] [--lumi] -- <lệnh>` chạy script với credential do broker cấp, nạp vào bộ nhớ của tiến trình con, không ghi ra file và không cần ai dán credential. Agent tự chạy script đọc hoặc sửa dữ liệu, không còn nhờ bạn chạy `!`. Xem [Chạy script với credential broker](/guides/broker-client#data-run).
  - Ghi trên staging mở cho cả lead và dev; ghi trên production chỉ lead.
  - `--write` chỉ chạy khi cùng lệnh đó đã dry-run thành công trong 60 phút trước, mỗi lần dry-run dùng được một lần; sửa script sau dry-run thì phải dry-run lại. Thiếu dry-run, lệnh thoát mã 7 và in đúng lệnh dry-run cần chạy.
  - Dry-run nhận credential chỉ đọc; output che giá trị credential và host. Lần chạy `--write` ghi log cạnh script trong `docs/scripts/<repo>/`.
  - `--lumi` cấp thêm credential MySQL và Qdrant của lumi-agent.
- **Thay đổi phá vỡ:** manifest bỏ 5 key của lumi-agent (`LUMI_MYSQL_URL_STG/PRD`, `QDRANT_URL_STG/PRD`, `LUMI_APP_URL_STG`). `zenify up --apply` xoá chúng khỏi `.zenify/.env`; từ bản này broker cấp chúng qua `zenify data run --lumi`.
- **Broker:** endpoint mới `POST /creds`; secret ghi và secret lumi là tuỳ chọn trong cấu hình broker. Người vận hành broker xem [Secret ghi và lumi](/guides/broker-host#secret-ghi-lumi), và nâng cấp broker trước khi teammate cài bản này.
- `zenify doctor` có mục `broker-creds`: kiểm tra broker cấp được credential staging; broker cũ chưa có `/creds` thì mục này bỏ qua.

## v0.39.0

*2026-10-17*

- **Stacked PR:** plan lớn có thể chia thành stack PR của GitHub; agent dựng các lớp, review từng lớp và mở stack bằng `gh stack`, người merge từ dưới lên. `zenify up --apply` cài extension `gh-stack`. Xem [ship](/workflows/ship).
- **Credential broker:** dev không còn tự điền credential database, PostHog và tài khoản E2E. Kit lấy chúng qua broker của team, xác thực bằng tài khoản GitHub (`gh auth login`) thuộc org ZenifyAIContactCenter. Broker chạy sau HTTPS tại `broker.zenify.vn`. Xem [Dùng credential broker](/guides/broker-client).
  - `zenify db-read` (cả `--prd`) chạy qua broker, giữ nguyên mọi giới hạn chỉ đọc.
  - Lệnh mới `zenify redis-read` đọc Redis staging hoặc production, chỉ các lệnh đọc, có giới hạn kích thước kết quả.
  - `zenify e2e login|run --role <role>` và `zenify visual check --role <role>` dùng tài khoản E2E riêng của từng dev, do broker cấp.
  - `zenify up --apply` đăng ký MCP `zenify-grafana` (Loki, Prometheus, Tempo) và `zenify-posthog` trong Claude Code. Skill `/znf:fix` dùng chúng để lấy log, trace và latency.
  - `zenify broker token [--refresh]` và `zenify broker headers` lấy JWT của broker.
  - Hai role theo team GitHub: team `leaders` là lead, đọc như trên; team `zenify-team` là dev, trên production chỉ đọc được cấu trúc và plan (`explain` ở mức `queryPlanner`, lệnh mới `db-read indexes|stats`, `count` không có `--filter`, schema SQL không có `WHERE` hay subquery, Redis metadata), lệnh khác báo `lead_only`. Staging, MCP và tài khoản E2E mở cho cả hai role. `zenify doctor` và `zenify broker token` in role. Xem [Lead và dev](/guides/broker-client#lead-dev).
  - `zenify doctor` có mục `broker`: kiểm tra đăng nhập GitHub, quyền org và kết nối tới broker.
- **Thay đổi phá vỡ:** manifest bỏ 10 key mà broker thay thế (database, PostHog, E2E). Sau `zenify up --apply`, các key này không còn trong `.zenify/.env`; `zenify doctor` báo chúng là "không còn dùng" nếu file còn giữ.
- **Skill dùng log, trace, metric và PostHog có chủ đích:** trang tham chiếu chung `evidence-sources` chỉ cho agent nguồn nào trả lời câu hỏi nào, thứ tự tra theo triệu chứng (lỗi, chậm, queue, phía người dùng), cách nối log → trace → dữ liệu, và các ngõ cụt đã đo (Tempo chỉ giữ khoảng 2 ngày, tham số đúng của từng tool). `/znf:hotfix`, `/znf:fix`, `/znf:explain-plan`, `/znf:ground`, `/znf:gate`, `/znf:scout`, `/znf:ship`, `/znf:cook` và `/znf:e2e` dùng trang này thay cho `docker logs` hay truy cập database trực tiếp. Xem [hotfix](/workflows/hotfix) và [Đọc dữ liệu thật](/guides/read-real-data).
  - `/znf:hotfix` có bước kiểm tra sau deploy: xác nhận production đã chạy bản sửa, so hai cửa sổ thời gian bằng nhau quanh lúc deploy, và nhóm lỗi còn lại theo client để tách tab cũ chưa tải lại khỏi bản sửa không có tác dụng.
  - `/znf:explain-plan` trên production chỉ đọc `count`, `stats` và plan không thực thi.
  - `zenify db-read --target audit` đọc database audit qua broker.
  - `zenify redis-read` báo chính xác khi kết quả dạng range bị cắt.
- **Sửa lỗi:**
  - `wt rm` fetch `origin` trước khi kiểm tra merge, và nhận cả base mà `wt new` đã tách worktree ra: hotfix đã merge vào `release<N>` gỡ được mà không cần `--force`. Worktree tạo bằng bản cũ hơn chưa ghi base này.
  - `zenify ui-verify record --screen` nhận tên có `/` (tên route) thay vì báo lỗi.
- Người vận hành dựng broker theo [Dựng host credential broker](/guides/broker-host): image `zenify-broker` sau nginx và cert certbot, audit log từng lệnh theo GitHub login.

## v0.38.1

*2026-10-16*

- Skill mới `/znf:where`: tóm tắt task đang tới đâu (mục tiêu, pha, đã xong kèm bằng chứng, đang làm, bước tiếp theo với lệnh kit, blocker). Skill đọc state thật: SDD ledger, branch, worktree, PR và các agent đã dispatch; agent chưa có báo cáo bị ghi là INCOMPLETE. Skill chỉ đọc, không sửa gì. Dùng khi quay lại sau khi nghỉ hoặc sau khi phiên bị nén. Xem [`/znf:where`](/reference/skills/where).

## v0.38.0

*2026-10-15*

- Hook `git-guard` nay đủ an toàn để agent chạy ở chế độ bypass hoặc auto. Luật chặn áp ở mọi chế độ quyền, kể cả khi lệnh được bọc trong `timeout`, `nohup`, `env`, `bash -c`, `sh -c` hay `eval`. Xem [Quan sát và an toàn](/guides/observe-and-safety).
  - Force push bị chặn trên mọi branch: `--force`, `-f`, `--force-with-lease`, refspec `+…`.
  - Merge PR bị chặn: `gh pr merge` và `gh api … PUT …/merge`.
  - Xoá worktree bằng `--force` bị chặn: `wt rm --force`, `zenify wt rm -f`, `git worktree remove --force`.
  - Đọc `.zenify/.env` qua Bash bị chặn, kể cả qua redirect (`cat < .zenify/.env`), qua glob `.zenify/*` và qua mở rộng biến `$X_PRD`.
  - Lệnh nhắc tới host của một URL `*_PRD` trong `.zenify/.env` bị chặn. Thông báo chặn chỉ nêu tên key, không nêu host.
  - Khi thật sự cần force push hay merge, bạn tự gõ lệnh ở dấu nhắc bằng `!`.
- **Thay đổi phá vỡ:** `zenify env exec` không còn đưa key nhóm production (`*_PRD`) cho tiến trình con. Script cần key production thì bạn tự chạy ở shell của mình. Xem [Credential dev với zenify env](/guides/env).
- `zenify db-read --prd` đọc dữ liệu production bằng `MONGO_URL_PRD` hoặc `MYSQL_URL_PRD`, vẫn giữ mọi giới hạn chỉ đọc của `db-read`. Agent tự đọc được production, bạn không cần gõ lệnh hộ. Xem [Đọc dữ liệu thật](/guides/read-real-data).
- Manifest thêm key `MYSQL_URL_PRD` vào nhóm production.

## v0.37.0

*2026-10-15*

- `zenify up --apply` dựng `<workspace>/.zenify/.env` thành file có sẵn khung. Mọi key trong manifest luôn có mặt; key chưa có giá trị là dòng `KEY=` trống. Mỗi key có hai dòng comment: mô tả, rồi `Lấy:` cách lấy · phạm vi · có phải secret không · repo. Key xếp theo bốn mục Staging → Tooling → Production → Custom. Giá trị sẵn có không đổi; comment tự viết dạng `# KEY=...` được giữ ở mục Custom. Xem [Credential dev với zenify env](/guides/env).
- Manifest thêm ba key production `MONGO_URL_PRD`, `AUDIT_MONGO_URL_PRD`, `REDIS_URL_PRD`. Wizard, form `zenify env set` lượt đầu và `zenify doctor` bỏ qua nhóm production.
- **Thay đổi phá vỡ:** `OPENROUTER_API_KEY` rời manifest và thành key custom. Ai đang có key này chạy một lần `zenify env set OPENROUTER_API_KEY --desc "<mô tả>"`, nếu không `zenify doctor` báo key custom thiếu mô tả.
- Key kit từng ghi nhận (`envKeys` trong `.zenify/manifest.json`) mà rời khỏi manifest bị xoá cùng giá trị ở lần `up --apply` kế tiếp, chỉ khi manifest là bản nhúng trong binary và binary không cũ hơn bản đã ghi. Chạy với `--manifest` hoặc trong một kit checkout thì key được giữ và in `env: giữ <tên> — <lý do>`. Key custom không bao giờ bị xoá.
- Dry-run và màn hình kế hoạch của wizard in trước `env: thêm …`, `env: xoá …`, `env: giữ …`, `env: custom thiếu mô tả …`, trước khi ghi gì.
- Key gán nhiều lần trong `.env`: kit lấy giá trị không trống cuối cùng, các giá trị khác giữ lại thành dòng `# KEY=...` và báo `env: giữ giá trị trùng của <tên> dưới dạng comment`.
- `zenify env`: key trống tính là chưa đặt; `list` gắn nhãn `custom` thay cho `local`; `set` một key ngoài manifest bắt buộc `--desc` (trừ khi file đã có mô tả), và từ chối giá trị trống.

## v0.36.0

*2026-10-15*

- **Thay đổi phá vỡ:** năm key `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE` được thay bằng một key `MYSQL_URL` dạng `mysql://user:pass@host:port/database`, cổng mặc định 3306. Ký tự `@ : / # ? %` và dấu cách trong user hoặc mật khẩu phải percent-encode, ví dụ `@` thành `%40`, `%` thành `%25`, dấu cách thành `%20`; hoặc nhập bằng `zenify env set MYSQL_URL`. `zenify up --apply` tự ghép năm key cũ trong `.zenify/.env` thành `MYSQL_URL` và xoá chúng; nếu `MYSQL_URL` đã có với giá trị khác, kit giữ nguyên và báo `env: bỏ qua MYSQL_URL — khác giá trị ghép từ MYSQL_*, xử lý tay`. `zenify db-read sql` báo lỗi nêu tên key và dạng đúng khi URL sai dạng, không in giá trị. Xem [Credential dev với zenify env](/guides/env#mysql-url).

## v0.35.0

*2026-10-14*

- **Thay đổi phá vỡ:** lệnh con `eval` của `zenify db-read` bị gỡ, vì lệnh này nhận code mongosh tự do. Thay bằng các lệnh có cấu trúc: `find`, `aggregate`, `explain` (cùng `collections`, `tables`, `doc`, `count`, `sql` đã có). Thay cho `distinct`: `zenify db-read aggregate <collection> --pipeline '[{"$group":{"_id":"$field"}}]'`. `--limit` mặc định 50, tối đa 200; một truy vấn chạy tối đa 30 giây. `sql` chỉ nhận một câu `SELECT`, `WITH`, `SHOW`, `DESCRIBE` hoặc `EXPLAIN` trong transaction READ ONLY, và từ chối comment, `\`, `INTO`, `FOR SHARE`, `LOCK IN SHARE MODE` ngoài chuỗi literal. Skill `/znf:ground` và `/znf:explain-plan` đã dùng cú pháp mới. Xem [Đọc dữ liệu thật](/guides/read-real-data).
- `zenify db-read explain` chạy không áp `--limit`, để executionStats phản ánh đúng số document truy vấn thật quét.
- Khi kết quả của `find`, `aggregate` hoặc `sql` bị cắt ở `--limit`, thông báo `… đã cắt ở <limit> dòng` ghi ra stderr, stdout chỉ chứa dữ liệu. Với `sql`, lệnh dừng tải ngay khi đủ `--limit` dòng, kể cả khi câu lệnh có `LIMIT` lớn hơn.
- Stdout của mọi lệnh `zenify db-read` tối đa 1 MiB. Quá mức này lệnh dừng ở ranh giới dòng, thoát mã 0 và ghi `… đã cắt ở 1048576 byte (giới hạn output)` ra stderr.
- `zenify db-read sql` từ chối ký tự xuống dòng nằm trong chuỗi literal; dùng `CHAR(10)` thay thế. Xuống dòng ngoài chuỗi vẫn dùng được. `SHOW CHARSET` và hàm `CHARSET()` chạy được.
- `zenify db-read` từ chối flag mà lệnh con không dùng, ví dụ `--filter` cho `doc` hay `--sort` cho `count`, thay vì lặng lẽ bỏ qua.
- `zenify db-read doc` in Extended JSON, ví dụ `{"$oid":"..."}` và `{"$date":"..."}`, giống `find` và `aggregate`.
- **Thay đổi phá vỡ:** credential cho tool dev và agent (`MONGO_URL`, `MYSQL_*`, `E2E_*`, `POSTHOG_API_KEY` và key riêng của repo) chuyển khỏi khối `env` của `settings.local.json` sang `<workspace>/.zenify/.env`, quyền 0600. `zenify db-read`, `zenify e2e`, `zenify visual` đọc từ file này; biến môi trường đã có sẵn trong tiến trình vẫn thắng nếu không rỗng. `zenify up --apply` tự chuyển key manifest sang file mới và xoá khỏi settings khi giá trị giống nhau; key có hai giá trị khác nhau được để nguyên và báo `env: bỏ qua <tên> — hai giá trị khác nhau, xử lý tay`. Hạ bản kit thì chạy `zenify env edit` rồi chép tay các key về khối `env`. Xem [Credential dev với zenify env](/guides/env).
- Lệnh mới `zenify env` (`list`, `set`, `edit`, `exec`): nhập ẩn key, mở file bằng `$EDITOR`, hoặc chạy script tay với key trong file mà output bị che giá trị secret từ 6 ký tự trở lên.
- `zenify up --apply` thêm luật deny `Read(//<workspace>/.zenify/.env)` (đường dẫn tuyệt đối) vào `~/.claude/settings.json` để agent không vô tình đọc file credential, ở mọi phiên kể cả phiên mở trong repo hay worktree; `zenify down --apply` gỡ luật của workspace đó và giữ luật bạn tự thêm. Wizard chỉ hỏi key toàn cục; key riêng của repo như `OPENROUTER_API_KEY` thêm bằng `zenify env set`.
- `zenify doctor` có mục `settings-env` (báo theo tên key còn nằm trong settings) và `env-file` (báo file quyền rộng hơn 0600 và key local chưa có mô tả).

## v0.34.1

*2026-10-14*

- `zenify up` nhận `message-hub-be` và `message-hub-web`, tên mới trên GitHub của `notification-hub-be` và `notification-hub-web`. Trước đây hai repo này hiện `SKIP:no-access` dù bạn vẫn có quyền. Đường dẫn trong manifest đổi thành `repos/message-hub-be` và `repos/message-hub-web`; bản clone cũ ở `repos/notification-hub-*` không được dời tự động, nên đổi tên thư mục bằng tay nếu không muốn `up` clone lại. Xem [Onboard workspace và repo](/guides/onboard-workspace).

## v0.34.0

*2026-10-14*

- Agent `spec-reviewer` mặc định chạy `opus` thay vì `fable`, để seat Team Standard không cạn hạn mức. Muốn soát spec bằng Fable, đặt `"ZNF_SPEC_REVIEW_MODEL": "fable"` trong khối `env` của `~/.claude/settings.json`; không đặt gì thì chạy `opus`. Xem [Chọn model](/concepts/model-routing#spec-reviewer).

## v0.33.2

*2026-10-14*

- `zenify wt new` đặt tên branch theo `git config user.name` (ví dụ `phuthuycoding/fix/<slug>`) thay vì phần trước `@` của `user.email`. Chưa đặt `user.name` thì vẫn lấy từ email, rồi tới `$USER`. Repo khai `user` trong `worktree.json` thì vẫn dùng giá trị đó. Ký tự không hợp lệ trong tên branch đổi thành `-`, nên `Nguyen An` thành `Nguyen-An`; muốn branch ngắn thì đặt `git config --global user.name <handle>`. Xem [Worktree theo slug](/concepts/worktree-per-slug).

## v0.33.1

*2026-10-14*

- `zenify up` không còn in hộp `WARNING: It looks like you are running 'npx playwright install'...` mỗi lần chạy. Khi Chromium đúng revision mà gói MCP ghim đã cài xong, `up` bỏ qua bước `npx playwright install` và in `playwright: chromium 1247 already installed for @playwright/mcp@0.0.83`. Thiếu bản nào thì vẫn cài như trước. Xem [Onboard workspace và repo](/guides/onboard-workspace).

## v0.33.0

*2026-10-13*

- `/znf:fix` dựng vòng phản hồi trước khi đoán: một lệnh có tên, đã chạy, đỏ đúng triệu chứng. Sau đó kit tự chọn seam, viết regression test đỏ rồi mới sửa, và báo cáo có dòng `Loop:` và `Regression:`. Secret, dữ liệu cá nhân khách hàng và id tenant/account được lọc ngay trong lệnh; request có tác dụng phụ chỉ chạy vào port của worktree hoặc môi trường local. Xem [/znf:fix](/workflows/fix).
- `/znf:hotfix` sửa và verify qua cùng vòng của `/znf:fix`. Bước chẩn đoán trên production chỉ dùng cách chỉ đọc, output đã lọc bí mật. Xem [/znf:hotfix](/workflows/hotfix).
- Implementer trong SDD theo luật chất lượng test: giá trị kỳ vọng lấy từ nguồn độc lập, chỉ mock ở ranh giới hệ thống, assert qua giao diện công khai. Phần không có giá trị kỳ vọng độc lập ghi `No TDD: <lý do>` kèm output chạy thật. Xem [/znf:cook](/workflows/cook).
- `/znf:review` đi qua bốn gate PRE, BUNDLE, REVIEW, VERIFY. T2 và T3 soát năm trục, trục `intent` thay `types` và so diff với ý định trong gói ship. T3 chạy cả năm reviewer bằng opus, không còn Workflow. Xem [Review và gate](/guides/review-and-gates).
- `/znf:ship` thêm mục "Rủi ro merge" vào thân PR. Gói review của `/znf:fix` và `/znf:hotfix` mang dòng `Loop:`/`Regression:`. Xem [/znf:ship](/workflows/ship).
- Brainstorming hỏi làm rõ theo lượt, tối đa 4 câu mỗi lượt, đáp án khuyến nghị đứng đầu, và kiểm module "sâu" khi thiết kế. Xem [/znf:cook](/workflows/cook).
- Description của skill và agent được rút gọn, tối đa 300 ký tự, có test chặn. Xem [Namespace znf](/concepts/znf-namespace).
- Skill mới `/znf:retro`, chỉ chạy khi bạn gõ. Nó gom bài học của phiên (điều phải đọc hay chạy mới biết, hoặc điều bạn đã sửa), mỗi bài một nơi lưu (check, rule, dòng `CLAUDE.md` hoặc memory). Bạn duyệt một lần đúng văn bản sẽ ghi; bí mật và dữ liệu cá nhân được loại bỏ. Xem [Chọn workflow](/workflows/).
- Asset của kit thống nhất "mở PR, không bao giờ merge". `finishing-a-development-branch` bỏ lựa chọn merge local, và `code-reviewer` không được gọi agent hay skill khác.

## v0.32.1

*2026-10-13*

- `zenify up` xoá file agent Codex mà kit đã gỡ (ví dụ `~/.codex/agents/architect.toml` còn sót sau v0.32.0) và in một dòng `removed retired Codex agent …`. Chỉ xoá file mang marker của kit; file agent bạn tự viết vẫn giữ nguyên.

## v0.32.0

*2026-10-13*

- Cách chọn model đổi sang quy tắc viết sẵn trong skill: implementer lấy model từ dòng `**Model:**` của task trong plan (mặc định `sonnet`, test fail lần 2 lên `opus`), investigator của `/znf:fix` theo vòng (`sonnet`, `opus`, rồi `opus` ở effort `xhigh`), review theo tier. Script `select-route` và skill `/znf:advisor` đã bị xoá; advisor native của Claude Code do từng người tự bật. Xem [Chọn model](/concepts/model-routing).
- Agent `architect` được thay bằng `spec-reviewer` (`model: fable`; model bị allowlist chặn thì chạy model kế thừa kèm cảnh báo, máy không có Fable chưa đo): brainstorming dispatch một lần sau bước tự soát spec, báo requirement thiếu, FR/SC không kiểm được, mâu thuẫn và tên chưa ground. Memo thiết kế giờ viết inline.
- `zenify up` và hook SessionStart xoá `ZNF_STRONG_MODEL` khỏi `~/.claude/settings.json` (mọi giá trị); `zenify rules lint` không còn cấm tên model mạnh trong `.md` của plugin.
- `zenify cost` không còn crash khi một file transcript không còn tồn tại (ví dụ symlink hỏng); số file bỏ qua được in ra.
- Knowledge hygiene (PR #154): lệnh `zenify docs sweep` (mặc định dry-run, `--dir` để chỉ rõ store) dọn plan đã build, handoff quá hạn và reference không còn link; `wt sweep` gọi nó sau khi dọn worktree đã merge. Hook `write-guard` (Claude) chặn ghi sai chỗ theo quy ước thư mục; `zenify doctor` thêm check `workspace-root` và `memory-index`. Research nháp ghi vào `docs/research`, ghi chú phiên ở lại `docs/notes`. Xem [Bố cục workspace](/concepts/workspace-layout).
- `/znf:ship` rút gọn thân PR: bỏ phần SC đã sửa và sổ deferred (PR #155).

## v0.31.1

*2026-10-12*

- `zenify up` ghim gói MCP Playwright ở `@playwright/mcp@0.0.83` và cài Chromium qua đúng gói đó, nên revision trình duyệt luôn khớp với MCP và chỉ tải lại khi kit nâng pin. Trước đây `up` đăng ký `@latest` rồi cài Chromium bằng một playwright khác, có ngày tải gần 600 MB mà MCP không dùng được.
- Registration cũ (`@latest` hoặc pin cũ, đúng cờ của kit) được `up` tự chuyển sang pin mới; registration có cờ riêng của bạn vẫn giữ nguyên. `zenify doctor` báo `mcp=legacy-default` cho tới khi chạy `up`.
- Tiến trình tải Chromium và output của `claude mcp add` / `codex mcp add` hiện thẳng trên terminal, không còn một dòng đứng yên nhiều phút.

## v0.31.0

*2026-10-12*

- Kit chạy được trên Codex, cạnh Claude Code. Khi máy có `codex` trên PATH, `zenify up` nối kit vào Codex: skill qua symlink `~/.agents/skills/znf` (cùng cây skill Claude đang dùng), hook vào `~/.codex/hooks.json` (sinh từ cùng bảng hook với Claude, xem cột Runtime ở trang [Hook](/reference/hooks)), 5 agent của kit thành `~/.codex/agents/*.toml`, và Playwright MCP đăng ký qua `codex mcp add`. Cuối lần chạy `up` in khối `Codex:` bốn dòng; máy không có `codex` chỉ thấy `Codex: không cài, bỏ qua`. `zenify down` gỡ đúng các mục kit đã ghi. Hướng dẫn: [Chạy với Codex](/guides/codex).
- Codex bỏ qua hook chưa trust mà không báo lỗi: sau `zenify up` lần đầu hoặc sau khi hook đổi, mở `codex`, gõ `/hooks` và chọn trust một lần.
- Trên Codex gọi skill bằng `$znf:<tên>` (ví dụ `$znf:fix`, `$znf:ship`); đối số là phần chữ đứng sau tên skill, nên các skill không còn dùng `$ARGUMENTS`. Codex không có hook `read-guard` và không có tool Workflow; `/znf:review` tầng T3 trên Codex chạy một lượt, không gom cụm.
- Chọn engine review: `zenify up --reviewer codex` đặt `ZNF_REVIEWER=codex` (mặc định `agent`), `zenify reviewer` in lựa chọn hiện tại và lý do. Với `codex`, `/znf:review` và bước review của `/znf:ship` chạy `codex review` thay cho agent reviewer; finding vẫn đi qua `review-verify` như trước.
- `/znf:ship` mở PR với body cố định: `## Nghiệm thu` (các SC của spec), `## Review còn mở` (các dòng deferred/parked từ ledger SDD), rồi phần tóm tắt và kiểm chứng.
- `zenify doctor` thêm năm check: `codex-hooks` (hook Codex của kit có đủ không), `reviewer` (engine đang chọn), `agents-md` (repo nào thiếu `AGENTS.md`), `shadow` (skill/agent/rule cá nhân trùng tên với kit), `context-budget` (tổng byte các file nạp mọi phiên, và từng `repos/<r>/CLAUDE.md` vượt 8 KiB, 200 dòng hoặc chứa cây thư mục). Các check này chỉ cảnh báo, không làm `doctor` fail.
- `route-log` ghi thêm `runtime` (`claude` hoặc `codex`) cho mỗi bản ghi.
- Repo kit có `AGENTS.md` cho Codex, trỏ về `ARCHITECTURE.md` thay cho `CLAUDE.md`.

## v0.30.0

*2026-10-07*

- `znf:ui-verifier` tự đăng nhập, không ai gõ mật khẩu vào trình duyệt nữa. Lệnh mới `zenify e2e login --url <app> | --port <port>` đăng nhập bằng `E2E_DOMAIN`, `E2E_EMAIL`, `E2E_PASSWORD` trong môi trường và ghi `~/.zenify/playwright/state.json` (quyền 0600). Agent nạp file này vào trình duyệt bằng `browser_set_storage_state`. Lệnh chỉ nhận origin localhost/loopback, và chạy được cả khi web trỏ API về hub local.
- Sau `zenify up`, verify UI trong auto mode không còn hỏi quyền:
  - Playwright MCP được đăng ký lại với `--isolated --caps=storage --output-dir ~/.zenify/playwright`. Đăng ký mặc định cũ của kit được tự chuyển sang dạng này; đăng ký bạn đã tự chỉnh được giữ nguyên, kèm cảnh báo.
  - `permissions.allow` trong `~/.claude/settings.json` có thêm các tool `mcp__playwright__*` mà verifier dùng, cùng `Bash(zenify e2e *)`, `Bash(zenify visual *)`, `Bash(zenify ui-verify *)`.
  - Nếu bật sandbox Bash, `sandbox.excludedCommands` có thêm `zenify e2e login *`.
  - Kit chỉ thêm những gì còn thiếu. Mục bạn đã tự xoá sẽ không bị thêm lại, và `zenify down` chỉ gỡ đúng các mục kit đã thêm.
- Nhiều session Claude chạy song song không còn giành nhau trình duyệt ("Browser is already in use"), vì mỗi session có trình duyệt riêng trong bộ nhớ.
- `zenify doctor` có thêm check `kit-permissions` (có `--fix`) và báo trạng thái đăng ký Playwright MCP.
- Bỏ `zenify release-report --unreleased`; `/znf:ship` không còn sinh lại `releases/unreleased.md` sau mỗi lần ship.

## v0.29.1

*2026-10-06*

- `zenify up` tự cài hook `git-guard`. Mỗi lần mở session cũng tự cài lại nếu thiếu. Trước đây `up` không cài guard, nên máy nào chưa tự chạy `zenify guard install` thì agent commit/push vào branch deploy mà không bị chặn. Máy đang thiếu guard sẽ tự có ở session đầu tiên sau khi cập nhật bản này.
- `zenify doctor` có hai check mới:
  - `git-guard`: báo khi thiếu hook. `zenify doctor --fix` tự gắn lại. Đây là check đầu tiên `--fix` thật sự sửa được.
  - `git-identity`: báo khi git không commit được vì thiếu `user.name`/`user.email`. Thiếu identity thì `zenify docs sync` không commit được và knowledge store ngừng đồng bộ.
- `zenify down` gỡ cả hook `git-guard`.
- `zenify guard install` (và việc tự cài ở trên) giữ nguyên phần còn lại của `~/.claude/settings.json`: không đổi thứ tự key, không escape `&&`, và không ghi đè khi `hooks` có kiểu lạ.
- `zenify docs sync` báo lý do thật khi git lỗi, thay vì chỉ `exit status 128`. Ví dụ: `commit error: exit status 128: Author identity unknown … fatal: unable to auto-detect email address`.
- `zenify wt new` đặt tên branch theo người đang chạy: phần trước `@` trong `git config user.email`, rồi `$USER` (Windows: `$USERNAME`), thay vì luôn là `namph/...`. Ký tự không hợp lệ trong tên branch được thay bằng `-`. `zenify wt config` in đúng giá trị này ở dòng `user`.

## v0.29.0

*2026-09-29*

- Port superpowers v6.4.2 vào znf, giữ nguyên ledger, `select-route`, fix-loop và UI-verify của kit.
- Plan ở mức quyết định. `writing-plans` ghi file, signature, đường dẫn và assertion của test cho mỗi bước; chỉ thuật toán khó mới kèm thân code. Mục "No Placeholders" đổi thành "What a Step Contains", thêm Review Focus và Proportion.
- Cổng duyệt plan mới trong `/znf:cook`:
  - Viết xong plan và chạy `/znf:analyze`, cook đưa đường dẫn plan cùng finding CRITICAL/HIGH rồi dừng chờ bạn duyệt.
  - Cook giờ có ba gate: thiết kế, spec, plan. Xem [/znf:cook](/workflows/cook).
- Mọi implementer của SDD chạy `sonnet`. Cùng một test fail lần 2 → `opus`, lần 3 → dừng và chạy investigator. SDD không còn đoán tier từ việc plan có chứa code hay không. Xem [Model routing](/concepts/model-routing).
- Implementer bắt buộc TDD: viết test từ assertion trong plan, thấy test fail rồi mới implement, chạy cả suite của project và báo tên test fail không do mình gây ra.
- `/znf:brainstorming`:
  - Bước đầu viết lại hiểu biết chung, tách điều bạn nói với điều agent giả định, và chờ bạn sửa.
  - Mỗi lần duyệt chỉ duyệt đúng stage vừa trình bày; duyệt ý tưởng không có nghĩa là duyệt artifact chưa tồn tại.
- Sửa lỗi script SDD không chạy sau `zenify skills sync`. Các file cài ở mode 0600, không có quyền thực thi; nay mọi script đều được gọi qua `bash`.
- Review và ship tính diff từ `git merge-base <baseRef> HEAD`. Khi base đã đi tiếp, review không còn thấy những file xoá giả.
- Reviewer doctrine:
  - Spec là bản định hướng.
  - Bảo mật, phân quyền, an toàn dữ liệu và vỡ hợp đồng luôn là lỗi.
  - Điều không đủ căn cứ để phán ghi vào khối `Declined:`, ngoài `findings[]`. Xem [Review và gate](/guides/review-and-gates).
- Script SDD `review-package` dừng với exit 3 khi range sai. `sdd-workspace` ghi marker `plan-path` để hai plan trùng tên file không dùng chung workspace, và báo lỗi thay vì lặp vô hạn khi `.znf/sdd` không ghi được.

## v0.28.0

*2026-09-28*

- Kit hỗ trợ repo Python.
- `mechanical-gate` của `/znf:review` và `/znf:ship` có nhánh Python cho repo có `pyproject.toml`, `setup.py`, `setup.cfg` hoặc `requirements*.txt`:
  - Các file `.py` thay đổi được kiểm cú pháp bằng `compileall`, ưu tiên `.venv/bin/python`, sau đó `python3`. Lỗi cú pháp là CRITICAL và chặn ship.
  - Nếu repo cấu hình ruff hoặc flake8, gate chạy thêm linter đó. Lint fail là HIGH.
  - Repo không cấu hình linter chỉ nhận một ghi chú LOW.
  - Trước đây một diff Python đi qua gate mà không được kiểm gì. Xem [Review và gate](/guides/review-and-gates).
- `zenify db-perf` nêu rõ những file nó chưa quét được. Scanner chỉ đọc query Mongo trong JS/TS; diff có file `.py`, `.java`, `.kt`, `.go`, `.rb`, `.php` hoặc `.cs` được liệt kê kèm dòng `coverage: unsupported stack`. Bản `--json` có thêm field `unscanned_files` và luôn ra JSON, kể cả khi diff không có query. `/znf:explain-plan` đọc field này, nên không còn báo "no query in this diff" cho một diff Python.
- `/znf:run` có cách chạy app không phải Node: `docker compose` hoặc ASGI chạy trực tiếp, kèm dấu hiệu nhận biết app đã sẵn sàng. `portEnv` chỉ có tác dụng khi app đọc biến đó.
- `manifest/repos.yaml` khai được `secretKeys` riêng cho từng repo:
  - `zenify up --apply` thêm các key còn thiếu (giá trị rỗng) vào `.claude/settings.local.json` của repo đó, cả khi repo đã onboard từ trước.
  - Giá trị có sẵn không bị đọc, đổi hay xoá.
  - Repo không thiếu key nào thì file không bị ghi lại.
  - `lumi-agent` khai `LUMI_MYSQL_URL_STG`, `LUMI_MYSQL_URL_PRD`, `QDRANT_URL_STG`, `QDRANT_URL_PRD`, `LUMI_APP_URL_STG` và `OPENROUTER_API_KEY`.
  - Xem [Onboard workspace và repo](/guides/onboard-workspace).

## v0.27.0

*2026-09-25*

- `zenify up` ghim model mặc định của workspace là `claude-opus-5-5[1m]` (Opus 5.5, 1M context) thay cho `claude-opus-4-8`. Giá trị này bị ghi đè mỗi lần apply, nên workspace nào chạy `zenify up` sau khi nâng kit đều chuyển sang model mới; `/model` hoặc `--model` theo từng phiên vẫn override được. Quay lại bản cũ trong một phiên: `/model claude-opus-4-8`. Xem [Chọn model](/concepts/model-routing).

## v0.26.0

*2026-09-21*

- Chọn model cho subagent theo đặc điểm task, không theo cảm nhận của agent: script `select-route` nhận site (`architect`, `investigator`, `implementer`, `reviewer`, `manual`) và số liệu (số repo, chạm contract chia sẻ, vòng thứ mấy, test fail mấy lần), trả về model và lý do; skill in lý do lên báo cáo. Xem [Chọn model](/concepts/model-routing).
- Model mạnh của máy đọc từ env `ZNF_STRONG_MODEL` (`fable` hoặc `opus`; thiếu hoặc khác → `opus`, không cảnh báo). `zenify up` ghi `opus` vào `~/.claude/settings.json` khi chưa có, `zenify down` gỡ giá trị mặc định. Teammate dùng Pro không bị dispatch tới model account không có. `zenify rules lint` từ chối tên model mạnh viết trực tiếp trong `.md` của plugin.
- Agent `architect` cho brainstorming tier architectural: chỉ dispatch khi một gate fire (từ 2 repo, contract chia sẻ, contract mới, vùng critical), một lần mỗi cook, viết memo mười mục; spec lấy Approach, Blast-radius, Flow, Rollback từ memo. Không gate nào fire thì memo viết inline. Xem agent `architect` (đã thay bằng `spec-reviewer` ở v0.32.0).
- Skill `/znf:advisor` (đã bỏ ở v0.32.0): ý kiến thứ hai từ model mạnh trong context mới cho câu hỏi bất kỳ, gọi tay hoặc bằng cụm chữ tường minh; thay cho việc đổi model cả session. Xem `/znf:advisor`.
- `/znf:fix` đếm `ROUND` trong ship-pack; investigator vòng 1 chạy sonnet, vòng 2 opus với context mới, từ vòng 3 model mạnh kèm log và các giả thuyết đã loại.
- SDD chọn tier implementer bằng độ cụ thể của plan (`SPEC=code` → haiku, prose → sonnet) và số lần cùng một test fail (lần 2 → opus kèm brief lỗi, lần 3 → dừng implement, chạy investigator). Escalation đổi tier, không đổi effort.
- Review T3 hai lần liên tiếp không shippable trên cùng branch → reviewer lên model mạnh; `zenify review-log` ghi thêm `branch` để tính streak.
- Lệnh `zenify route-log` đọc log calibrate ở `.znf/route-log/` (`--since`, `--json`): số route theo site, gate đã fire, tỉ lệ architect đổi quyết định. `zenify cost` in thêm token thinking của session chính và các mức effort đã gặp. Xem [`zenify route-log`](/reference/cli/zenify_route-log).
- Fork skill (`research`, `analyze`, `scout`, …) ghim sonnet thay vì kế thừa model session; `zenify analyze` ghi metric của plan vào route-log; writing-plans nhắc ở bước tách task cách ghi plan đủ cụ thể để implementer chạy tier rẻ.

## v0.25.0

*2026-09-21*

- Bảy coding skill theo stack (`mongo-data-safety`, `sql-data-safety`, `mongoose-modeling`, `service-integration`, `express-service-patterns`, `nestjs-patterns`, `react-patterns`) chuyển vào plugin `znf`, gọi bằng `znf:<name>`. Không còn chép per-repo: `zenify skills install` giờ **gỡ** bản cũ trong `.claude/skills/` theo manifest, giữ skill dev tự thêm. Xem [Namespace znf](/concepts/znf-namespace).
- Route domain skill trong workflow thay vì trông vào lời nhắc: bảng `_shared/skill-routing` ánh xạ tín hiệu trong task (chạm Mongo, SQL, schema, pub/sub, Express, NestJS, React) sang skill; mỗi task trong plan khai tag `_Skills:`; `/znf:ground` thêm bước route; implementer của SDD invoke skill trước lần sửa đầu.
- `zenify analyze` báo HIGH `missing-skills` (task thiếu tag hoặc tag rỗng) và `unknown-skill` (tên không phải `none`, `znf:<skill>` hay `<repo>-conventions`). Xem [Spec, plan và kiểm tra](/guides/spec-and-plan).
- `/znf:research`: brief gửi worker và verifier luôn tiếng Anh, kể cả khi câu hỏi gõ tiếng Việt. Worker ghi claim tiếng Anh, chỉ báo cáo cuối theo ngôn ngữ project.
- Bỏ task list của harness (`TodoWrite`/`TaskCreate`) khỏi mọi skill: mỗi lệnh là một lượt API riêng, đo được 7 đến 18% chi phí lượt tool. Tiến độ theo ledger file của skill. Xem [Ngân sách context](/concepts/context-budget#task-list-của-harness-tắt).
- Quy tắc chung trong `artifact-style`: văn bản máy đọc (brief subagent, commit, memory, rule) luôn tiếng Anh; lead chạy fork không được truyền ngôn ngữ của người dùng xuống worker.

## v0.24.0

*2026-09-20*

- Skill `/znf:research`: research có kiểm chứng, chạy tách context. Worker tìm và trích dẫn, verifier tải lại từng URL, chỉ hàng `VERIFIED` được dùng. Báo cáo ghi vào `docs/reference/`, trả về tóm tắt ≤ 40 dòng. Xem [Research có kiểm chứng](/concepts/research).
- Brainstorming thêm bước **Research check**: checklist năm mục, một mục đúng thì gọi `/znf:research` trước khi đề xuất phương án.
- `/znf:cook` chạy trong một session từ đầu tới ship. Bỏ hai điểm dừng đổi session sau plan và sau implement.
- Site có checklist [Cắt release](/guides/release#cắt-release-cho-zenify-kit) và trang Changelog này. Rule `kit-release-docs` trong knowledge store yêu cầu cập nhật trang viết tay trong cùng PR đổi hành vi.

## v0.23.0

*2026-09-20*

- Hook `read-guard` chặn lệnh Read quá lớn trước khi nó vào context, kèm gợi ý đọc từng phần.
- Meter chỉ hỏi một lần khi session nặng, rồi im lặng.
- Statusline tô màu `ctx%`: vàng từ 50, đỏ từ 80.
- Mọi dispatch subagent trong skill đều nêu cap trả về 40 dòng.
- Trang [Ngân sách context](/concepts/context-budget) giải thích ba đòn bẩy trên.

## v0.22.2

*2026-09-19*

- `zenify cost --by-skill`: bảng token theo skill (số lần gọi, token, Mtok mỗi lần), tính cả token của session chính.
- Sửa đếm trùng token: transcript ghi một dòng cho mỗi content block nên số cũ cao khoảng 2,4 lần.

## v0.22.1

*2026-09-19*

- Điểm dừng đổi session trong cook hỏi trước thay vì ép chuyển.
- Model gọi lại được `cook`, `ship`, `run`, `sweep` (bản trước vô tình chặn).
- Gỡ skill `onboard-project` không còn dùng.

## v0.22.0

*2026-09-18*

- Token diet: mỗi skill tối đa 8 KB, skill hỏi đáp chạy fork, bảng harness-tools dùng chung.
- Cook ba pha, review hai lớp, subagent mặc định sonnet, meter đưa lời khuyên khi session nặng.
- Lệnh `zenify cost`: token đi đâu, đọc từ transcript Claude Code trên máy.
