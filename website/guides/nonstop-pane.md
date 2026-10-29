---
title: "Pane nonstop: điều phối cả ngày trong một terminal"
---

# Pane nonstop

Khi chạy [`/znf:nonstop`](/reference/skills/nonstop), chat chính chỉ còn là nơi điều phối và trả lời quyết định. Trạng thái task, PR và thống kê nằm trong một pane bên cạnh chat (hàng tab ở trên, khu `Thống kê` cố định ở đáy), tự cập nhật, không tốn lượt chat nào.

## Bật pane

Pane là một mod trong plugin `znf`, có sẵn sau `zenify skills sync`. Không cần cài thêm.

- Gõ `/znf:nonstop` (hoặc `/znf:nonstop continue`): pane `Nonstop` mở ra cạnh chat. Nếu còn bốn pane `nonstop-*` của bản cũ, chúng được đóng.
- `/nonstop-pane`: mở hoặc đóng pane bằng tay.
- Pane chỉ dock cạnh chat khi Claude Code chạy fullscreen (`"tui": "fullscreen"` trong `~/.claude/settings.json`). Không fullscreen thì pane nằm phía trên ô prompt.
- Session thường không mở pane và không poll gì. Pane không tự mở khi session bắt đầu; sau khi khởi động lại Claude Code, gõ `/znf:nonstop continue` để mở lại.

## Bốn tab trong một pane

Hàng tab `Need Human | In Progress | Pending | Done` do pane tự vẽ: bấm vào tab; tab đang chọn có dấu `▸` trước tên, các tab còn lại mờ (mặc định Need Human). Nội dung tab dài hơn khoảng trống thì cắt và ghi `+N dòng` (dim).

| Tab | Nội dung |
|---|---|
| **Need Human** | Câu hỏi chờ bạn (task `blocked`, dòng `Needs you:`) và PR sẵn sàng merge |
| **In Progress** | Dòng `▶ đang chạy N worker` (spinner) hoặc `⏸ tạm dừng`; task `RUNNING`, `QUEUED`; khu `ĐANG WATCH` liệt kê mọi PR còn mở với `CI · Zenie · thread mở/tổng`; dòng đếm và cảnh báo ⚠ dòng ledger sai format |
| **Pending** | Task `deferred` (khu `HOÃN`, kèm `tới <ngày>`, sắp theo ngày rồi priority) và task `carry-over` (khu `CARRY-OVER`, sắp theo priority); khu rỗng thì ẩn |
| **Done** | Task `done`, PR đã merge hoặc đóng. PR đóng vẫn được poll: mở lại thì nó quay về `ĐANG WATCH` |

Dưới mọi tab là khu `Thống kê` cố định, ngăn bởi một dòng kẻ, tối đa 5 dòng: thang size, hôm nay, 7 ngày, review kèm tier, và một dòng ⚠ (task done chưa ghi estimate, hoặc dòng estimates sai format, hoặc lỗi thống kê). Chưa có `estimates.md` thì hiện `chưa có estimates.md`. Dòng cuối pane `Xem thêm thông tin` mở định nghĩa các chỉ số (trang nằm trong knowledge store, repo private của team).

Trên mọi tab:

- Task và câu hỏi có priority `P0`–`P3` ở đầu dòng, sắp từ P0.
- Bấm vào task id (ví dụ `t14`) để mở rộng đầy đủ dòng đó, bấm lại để thu gọn.
- PR (`owner/repo#n`) và issue là link, bấm để mở trên browser.

## Status line

Dòng trạng thái của Claude Code hiện số liệu của ngày, mục bằng 0 thì ẩn (trừ In Progress):

```text
nonstop ▶ · In Progress: 1/2 · Need Human: 5 · Pending: 2 · Done: 5/8 · Ready: 2 · CI ✗: 1 · Threads: 3 · 11 pts · hit 71%
```

`▶` khi có task đang chạy, `⏸` khi không. `Done: d/t` đếm trên task của ngày (không tính `deferred` và `carry-over`). `Ready` là số PR sẵn sàng merge, `CI ✗` là số PR có CI đỏ, `Threads` là tổng thread mở của PR chưa merge, `pts` và `hit` lấy từ thống kê hôm nay.

## Merge từ pane

Một PR chỉ hiện ở mục `CHỜ MERGE` của tab Need Human khi đủ cả ba điều kiện: `mergeStateStatus` là CLEAN hoặc UNSTABLE, check `Zenie Review` đã pass, và không còn review thread nào mở. PR không có check Zenie thì không có nút merge.

- `[c]` copy lệnh `gh pr merge -R <owner/repo> <n> --merge`.
- `[merge]` merge bằng `gh`, cần bấm hai lần: lần đầu nút đổi thành `xác nhận → <base>?`, lần hai trong 5 giây mới chạy. Lần hai đọc lại PR ngay lúc bấm: chưa sẵn sàng (vd có thread mới mở) hoặc base đã đổi khác base trong nút thì không merge và hiện toast `chưa merge được: …`; sẵn sàng thì merge kèm `--match-head-commit <SHA vừa đọc>`, nên commit mới đẩy lên sau lần đọc đó sẽ chặn merge. Trong lúc merge, nút hiện `đang merge…` và bấm thêm không có tác dụng. Lỗi của `gh` hiện thành toast `merge lỗi: …`.

Chỉ người bấm mới merge. Agent không gọi đường này. PR vào `staging` hay `releaseNN` thì merge là deploy, nên đọc kỹ base trong nút xác nhận.

## Ledger mà pane đọc

Pane đọc thẳng `.zenify/nonstop/progress.md` (workspace) hoặc `<repo>/.znf/nonstop/progress.md`, mỗi 2 giây khi file đổi. Skill nonstop bắt orchestrator ghi section cuối theo dòng 10 trường:

```text
id · priority · repo · source · issue · size · skill · worker · status · PR/evidence
```

Dòng bảng hoặc heading `###` bị đếm là sai format. Khi re-entry gặp section dạng bảng, orchestrator viết lại sang dòng trước khi dispatch. Chi tiết trong `references/ledger-format.md` của skill.

## Đo thời gian và thống kê

Mod tự ghi `durations.log` cạnh ledger: mỗi lượt worker hoặc reviewer chạy xong, một dòng `<thời điểm> · <task id> · work|review · <ms>`. Thời gian chờ bạn trả lời không tính, vì nó nằm giữa hai lượt chạy.

`zenify nonstop stats` là nguồn duy nhất cho thang size và thống kê. Orchestrator chạy nó khi lập plan sáng và khi đóng ngày; pane chạy lại nó khi `estimates.md`, `durations.log` hoặc ngày đổi (lần chạy lỗi thì thử lại ở tick sau) để vẽ khu `Thống kê`:

```text
$ zenify nonstop stats
scale (default, 7 tasks): S<5m M<20m L<60m
today: 0/0 hit, 0 over, 0 under, points 0
review: 0m (0%)
tiers 7d: T1 0 · T2 0 · T3 0
7 days: hit 29%, 9.0 points/day
```

- Thang size học từ thời gian `work` thật: có từ 20 task đo được thì ngưỡng S/M/L là phân vị 25/50/90 của 60 task gần nhất; ít hơn thì dùng mặc định 5/20/60 phút.
- Thời gian review đứng riêng, không ảnh hưởng thang.
- Luật hiệu chỉnh (hiện ở output CLI, không ở khu `Thống kê` của pane) (`est lên/xuống 1 size`, tier thường gặp) chỉ đọc 30 dòng gần nhất của `estimates.md`; thống kê ngày và tuần dùng toàn bộ.
- Độ phức tạp dùng lại hai phân loại có sẵn: path của [`znf:brainstorming`](/reference/skills/brainstorming) (spike, bounded, architectural) và tier T1/T2/T3 của [`znf:review`](/reference/skills/review).
- `--json` cho pane và script; `--dir` trỏ tới thư mục ledger khác.

Xem thêm [`zenify nonstop stats`](/reference/cli/zenify_nonstop_stats).
