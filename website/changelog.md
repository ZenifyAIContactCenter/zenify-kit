---
title: Changelog
---

# Changelog

Mỗi release một mục, viết cho người dùng kit, không phải danh sách commit. Mục của một bản được viết **trước khi tag**, trong PR docs của lần cắt release, theo [Cắt release cho zenify-kit](/guides/release#cắt-release-cho-zenify-kit). Workflow release từ chối tag chưa có mục ở đây, và dùng chính mục đó làm nội dung GitHub Release, nên trang này không thể cũ hơn bản đang phát hành.

Bản trước `v0.22.0`: xem [GitHub Releases](https://github.com/ZenifyAIContactCenter/zenify-kit/releases).

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
