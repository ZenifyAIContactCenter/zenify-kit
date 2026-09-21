---
title: Changelog
---

# Changelog

Mỗi release một mục, viết cho người dùng kit, không phải danh sách commit. Mục của một bản được viết **trước khi tag**, trong PR docs của lần cắt release, theo [Cắt release cho zenify-kit](/guides/release#cắt-release-cho-zenify-kit). Workflow release từ chối tag chưa có mục ở đây, và dùng chính mục đó làm nội dung GitHub Release, nên trang này không thể cũ hơn bản đang phát hành.

Bản trước `v0.22.0`: xem [GitHub Releases](https://github.com/ZenifyAIContactCenter/zenify-kit/releases).

## v0.26.0

*chưa cắt*

- Chọn model cho subagent theo đặc điểm task, không theo cảm nhận của agent: script `select-route` nhận site (`architect`, `investigator`, `implementer`, `reviewer`, `manual`) và số liệu (số repo, chạm contract chia sẻ, vòng thứ mấy, test fail mấy lần), trả về model và lý do; skill in lý do lên báo cáo. Xem [Chọn model](/concepts/model-routing).
- Model mạnh của máy đọc từ env `ZNF_STRONG_MODEL` (`fable` hoặc `opus`; thiếu hoặc khác → `opus`, không cảnh báo). `zenify up` ghi `opus` vào `~/.claude/settings.json` khi chưa có, `zenify down` gỡ giá trị mặc định. Teammate dùng Pro không bị dispatch tới model account không có. `zenify rules lint` từ chối tên model mạnh viết trực tiếp trong `.md` của plugin.
- Agent `architect` cho brainstorming tier architectural: chỉ dispatch khi một gate fire (từ 2 repo, contract chia sẻ, contract mới, vùng critical), một lần mỗi cook, viết memo mười mục; spec lấy Approach, Blast-radius, Flow, Rollback từ memo. Không gate nào fire thì memo viết inline. Xem [agent architect](/reference/agents/architect).
- Skill `/znf:advisor`: ý kiến thứ hai từ model mạnh trong context mới cho câu hỏi bất kỳ, gọi tay hoặc bằng cụm chữ tường minh; thay cho việc đổi model cả session. Xem [`/znf:advisor`](/reference/skills/advisor).
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
