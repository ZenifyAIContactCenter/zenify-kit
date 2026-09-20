---
title: Changelog
---

# Changelog

Mỗi release một mục, viết cho người dùng kit, không phải danh sách commit. Mục của một bản được viết **trước khi tag**, trong PR docs của lần cắt release, theo [Cắt release cho zenify-kit](/guides/release#cắt-release-cho-zenify-kit). Workflow release từ chối tag chưa có mục ở đây, và dùng chính mục đó làm nội dung GitHub Release, nên trang này không thể cũ hơn bản đang phát hành.

Bản trước `v0.22.0`: xem [GitHub Releases](https://github.com/ZenifyAIContactCenter/zenify-kit/releases).

## v0.24.1

*chưa cắt*

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
