---
title: "Pane nonstop: điều phối cả ngày trong một terminal"
---

# Pane nonstop

Khi chạy [`/znf:nonstop`](/reference/skills/nonstop), chat chính chỉ còn là nơi điều phối và trả lời quyết định. Trạng thái task, PR và thống kê nằm ở bốn tab bên cạnh chat, tự cập nhật, không tốn lượt chat nào.

## Bật pane

Pane là một mod trong plugin `znf`, có sẵn sau `zenify skills sync`. Không cần cài thêm.

- Gõ `/znf:nonstop` (hoặc `/znf:nonstop continue`): bốn tab mở ra cạnh chat.
- `/nonstop-pane`: mở hoặc đóng cả bốn tab bằng tay.
- Pane chỉ dock cạnh chat khi Claude Code chạy fullscreen (`"tui": "fullscreen"` trong `~/.claude/settings.json`). Không fullscreen thì pane nằm phía trên ô prompt.
- Session thường không mở pane và không poll gì. Khi Claude Code khởi động lại giữa một ngày nonstop còn task `running`, pane tự mở lại.

## Bốn tab

| Tab | Nội dung |
|---|---|
| **Need Human** | Câu hỏi chờ bạn (task `blocked`, dòng `Needs you:`) và PR sẵn sàng merge |
| **In Progress** | Dòng `▶ đang chạy N worker` (spinner) hoặc `⏸ tạm dừng`; task `RUNNING`, `QUEUED`; khu `ĐANG WATCH` liệt kê mọi PR còn mở với `CI · Zenie · thread mở/tổng`; dòng đếm và cảnh báo ⚠ dòng ledger sai format |
| **Pending** | Task `deferred` (khu `HOÃN`, kèm `tới <ngày>`, sắp theo ngày rồi priority) và task `carry-over` (khu `CARRY-OVER`, sắp theo priority); khu rỗng thì ẩn |
| **Done** | Task `done`, PR đã merge hoặc đóng, khu `THỐNG KÊ` |

Trên mọi tab:

- Task và câu hỏi có priority `P0`–`P3` ở đầu dòng, sắp từ P0.
- Bấm vào task id (ví dụ `t14`) để mở rộng đầy đủ dòng đó, bấm lại để thu gọn.
- PR (`owner/repo#n`) và issue là link, bấm để mở trên browser.

## Merge từ pane

Một PR chỉ hiện ở mục `CHỜ MERGE` của tab Need Human khi đủ cả ba điều kiện: `mergeStateStatus` là CLEAN hoặc UNSTABLE, check `Zenie Review` đã pass, và không còn review thread nào mở. PR không có check Zenie thì không có nút merge.

- `[c]` copy lệnh `gh pr merge -R <owner/repo> <n> --merge`.
- `[merge]` merge bằng `gh`, cần bấm hai lần: lần đầu nút đổi thành `xác nhận → <base>?`, lần hai trong 5 giây mới chạy. Trong lúc merge, nút hiện `đang merge…` và bấm thêm không có tác dụng. Lỗi của `gh` hiện thành toast `merge lỗi: …`.

Chỉ người bấm mới merge. Agent không gọi đường này. PR vào `staging` hay `releaseNN` thì merge là deploy, nên đọc kỹ base trong nút xác nhận.

## Ledger mà pane đọc

Pane đọc thẳng `.zenify/nonstop/progress.md` (workspace) hoặc `<repo>/.znf/nonstop/progress.md`, mỗi 2 giây khi file đổi. Skill nonstop bắt orchestrator ghi section cuối theo dòng 10 trường:

```text
id · priority · repo · source · issue · size · skill · worker · status · PR/evidence
```

Dòng bảng hoặc heading `###` bị đếm là sai format. Khi re-entry gặp section dạng bảng, orchestrator viết lại sang dòng trước khi dispatch. Chi tiết trong `references/ledger-format.md` của skill.

## Đo thời gian và thống kê

Mod tự ghi `durations.log` cạnh ledger: mỗi lượt worker hoặc reviewer chạy xong, một dòng `<thời điểm> · <task id> · work|review · <ms>`. Thời gian chờ bạn trả lời không tính, vì nó nằm giữa hai lượt chạy.

`zenify nonstop stats` là nguồn duy nhất cho thang size và thống kê. Orchestrator chạy nó khi lập plan sáng và khi đóng ngày; tab Done chạy nó để hiện `THỐNG KÊ`:

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
- Độ phức tạp dùng lại hai phân loại có sẵn: path của [`znf:brainstorming`](/reference/skills/brainstorming) (spike, bounded, architectural) và tier T1/T2/T3 của [`znf:review`](/reference/skills/review).
- `--json` cho pane và script; `--dir` trỏ tới thư mục ledger khác.

Xem thêm [`zenify nonstop stats`](/reference/cli/zenify_nonstop_stats).
