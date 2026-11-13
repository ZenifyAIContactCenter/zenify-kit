---
title: "Pane track: đo thời gian, token và chất lượng của từng lượt làm việc"
---

# Pane track

`track` ghi lại mỗi lượt `znf:cook`, `znf:fix`, `znf:hotfix` và `znf:ship` để bạn thấy việc tốn bao lâu, tốn bao nhiêu token và ship có suôn sẻ không. Số liệu hiện trong một pane bên cạnh chat và một status line ở đáy, tự cập nhật, không tốn lượt chat nào. Tính năng mặc định tắt.

Khi tính năng bật, mỗi lần một trong các skill đó (hoặc `znf:ci-watch`) được nạp, dù do agent tự gọi hay do bạn gõ `/znf:…`, plugin gắn kèm đúng các bước `zenify track` của skill ấy (lấy từ `skills/_shared/track.md`) vào ngữ cảnh của agent. Nhờ vậy agent ghi track mà không cần tự nhớ đọc `track.md`.

## Bật và tắt

```bash
zenify feature track on
zenify feature track off
```

Ngay trong Claude Code, gõ `/config` rồi bật dòng `Dev track` của plugin `znf` để làm tương tự (xem [Settings](#settings)); status line tự cập nhật trong vài giây. Lệnh áp dụng cho cả workspace. Khi bật, digest đầu session nhắc agent đọc `_shared/track.md`; agent gọi các lệnh [`zenify track`](/reference/cli/zenify_track) để ghi số liệu, bạn không cần gõ gì. Digest còn kèm một dòng tóm tắt việc đang mở, ví dụ `track: 2 carried · plan 1/4 done · 3 PR waiting · 21 backlog` (mục bằng 0 thì ẩn, không có gì mở thì không in dòng này), để agent biết trạng thái ngay từ đầu session. Dòng này chỉ đọc file trong `.zenify/track/` (log, cache PR đã merge, cache backlog), không gọi `gh`, nên số PR và số issue GitHub trong backlog chỉ đúng tới lần chạy stats gần nhất. Mục plan đã huỷ không tính vào tổng. Tắt thì mọi lệnh ghi của `zenify track` in `track: off` và không ghi.

Pane là một mod trong plugin `znf`, có sẵn sau `zenify skills sync`. Gõ `/track-pane` để mở hoặc đóng pane. Pane cũng tự mở khi prompt của bạn là `cook`, `fix`, `hotfix` hoặc `ship` (có hoặc không có tiền tố `znf:`), nếu dòng `Track pane` trong `/config` là `auto` (mặc định). Pane chỉ tự mở theo prompt bạn gõ, không mở khi agent tự gọi skill giữa chừng (vd `znf:fix` gọi tiếp `znf:ship`). Bạn đã đóng pane thì nó không tự mở lại cho tới lần `/clear` hoặc compact kế tiếp; muốn mở lại ngay thì gõ `/track-pane`. Pane, lệnh, các dòng `znf` trong `/config` và status line chỉ có khi Claude Code mở trong một zenify workspace (có `.zenify/manifest.json` ở thư mục hiện tại hoặc thư mục cha); ngoài workspace chúng ẩn hết. Session đã mở trong workspace mà sau đó `cd` sang thư mục ngoài (vd knowledge store `~/.zenify/knowledge`) vẫn ghi track vào workspace ban đầu; riêng `/clear` hay compact khi cwd đang ở ngoài thì tính là ngoài workspace.

## Status line

Status line luôn hiện, kể cả khi pane đóng. Toàn bộ chữ trên pane và status line là tiếng Anh.

- Khi tắt: `⏸ track off · enable: /config → znf`
- Khi bật và tab này đang có việc mở: `track ▶ <id> · <phase> · <wall>m/<est> (agent <n>m) · wait <n>m · Today: done/started · PR n (m merged) · hit n%`
- Khi bật và tab này chưa có việc: `track ⏸ · Today: done/started · ...`
- Khi bật, tab này chưa có việc và workspace chưa onboard: `track ⏸ · Tip: onboard the agent to pick work — type "hey claude onboard track"`
- Số đầu tiên là wall time (đồng hồ thật từ lúc bắt đầu), kèm `<est>` nếu việc có size; `(agent <n>m)` là thời gian agent thật sự làm, chỉ hiện khi lớn hơn 0. Chi tiết ở [Thời gian wall và thời gian agent](#thời-gian-wall-và-thời-gian-agent).
- `hit n%` chỉ hiện khi đã có ít nhất 5 việc có est.
- Khi tab này chưa có việc mở, tip việc cần xử lý ngay đứng trước các tip bên dưới: PR có CI đỏ, rồi PR còn thread chưa resolve, rồi PR bị yêu cầu sửa, rồi PR đã sẵn sàng merge, rồi việc carried over đã mở từ 5 ngày trở lên hoặc bị stale (`Tip: <id> open <n> days — continue or cancel it`).
- Tip theo ngữ cảnh: mỗi lúc tối đa một tip, nối cuối dòng bằng ` · `, chọn theo thứ tự ưu tiên (onboard ở trên trước; rồi carried over, chưa có plan, Backlog):
  - tab này không có id mở và còn việc làm dở từ các ngày trước: `Tip: N items carried over — type "hey claude continue"`
  - tab này không có id mở, đã onboard và hôm nay chưa có plan: `Tip: no plan today — type "/znf:track plan"` (xem [Daily plan](#daily-plan))
  - tab này không có id mở và Backlog còn việc: `Tip: N items in Backlog — type "/znf:track ls"`
  - có id mở, chưa có estimate và đã chạy từ 5 phút: `Tip: no estimate yet — type "hey claude estimate this"`
  - có id mở, đã vượt ngưỡng của size est: `Tip: over est M (50m/45m) — split the work or re-estimate`
  - còn PR đã mở (bất kỳ ngày nào) chưa merge: `Tip: N PRs waiting to merge — type "hey claude check PR"`

Mục nào bằng 0 hoặc chưa biết thì ẩn, riêng `Today` luôn hiện. Đọc số liệu lỗi thì status line hiện `⚠ track error: <nguyên nhân>`. Số liệu được tải ở nền: trong lúc `zenify track stats` đang chạy (có thể quá 20s khi phải hỏi `gh` từng PR đang mở), status line vẫn giữ số liệu cũ, thêm dấu `⟳` ở đầu nếu lần chạy quá 1s, rồi tự cập nhật khi lần chạy xong; chỉ báo lỗi khi lần chạy hỏng hoặc quá 2 phút.

## Năm tab

Đổi tab bằng chuột chỉ được khi Claude Code chạy fullscreen. Trên màn hình thường (pane nằm trên ô prompt), terminal không báo click về cho pane: bấm `ctrl+x tab` để pane nhận bàn phím, rồi bấm số `1`–`5` (Kitchen, Today, Estimate, Quality, Backlog), hoặc `Tab` và `Enter`; `Esc` quay lại prompt. Khi pane chưa nhận bàn phím, dưới hàng tab có dòng mờ `ctrl+x tab, then 1–5 to switch tabs`.

| Tab | Nội dung |
|---|---|
| **Kitchen** | Tab mặc định, giống nhau ở mọi session. Thứ tự từ trên xuống: `🗓 Plan` (chỉ khi hôm nay đã có plan, xem [Daily plan](#daily-plan)), `📌 Carried over` (xem [Việc làm dở và ngày làm việc cuối](#việc-làm-dở-và-ngày-làm-việc-cuối)), `🥘 Cooking`, `🔥 Rolling boil`, `🍳 Cooked`. `🔥 Rolling boil`: các tab Claude Code đang chờ bạn, mỗi dòng `- <❓ hoặc 🔐> <thời gian chờ> <tab>` (`45s`, `2m`, `2h`, làm tròn xuống); không có thì hiện `no tab is waiting`. `🍳 Cooked`: các việc đã xong hôm nay, mỗi dòng `- <icon> <HH:MM> <brief>` (có est thì thêm ` · M→52m`: est và wall time thật; wall đã chạm mốc của size thì thêm ` ⚠`; giờ máy; icon cho biết kết quả: ✅ done và mọi PR đã merge, ⏳ done nhưng còn PR chưa merge, 🚀 pushed, ⛔ cancelled, ↩️ rollback), mỗi PR một dòng `  🔗 <repo>#<n>` là link bấm được tới PR; chưa có thì hiện `nothing finished today`. Lấy tiêu đề lỗi thì có dòng `⚠ title error: <nguyên nhân>` |
| **Today** | Hôm nay của cả workspace: `Done`, `started`, `Cancelled`, `Rollback` (mục bằng 0 thì ẩn), `Time <wall>m · agent <n>m`, `SDD tasks` (chỉ khi hôm nay có cook xong), `Rounds`, `PR n opened · m merged` và mỗi PR đã mở chưa merge một dòng link (xem [PR cần hành động](#pr-cần-hành-động)), `Plans written today` theo repo; danh sách `Running` gom theo session, tab hiện tại ghi `(this tab)`; việc bị bỏ dở ghi `stale` |
| **Estimate** | Với mỗi loại (cook, fix, hotfix): est đúng/thấp/cao so với thực tế (`hit`, `under`, `over`); hotfix và `task` (việc ops, xem [Việc ops](#việc-ops-kind-task)) hiện số việc, trung vị và p90 (block `task` chỉ hiện khi đã có việc `task` xong; `task` không góp vào thang S/M/L); `agent time by phase` là tỉ lệ thời gian agent theo giai đoạn |
| **Quality** | Với mỗi loại: tỉ lệ ship bị chặn và bước chặn nhiều nhất, finding C/H/M/L trung bình, số vòng fix loop, tỉ lệ CI đỏ lần đầu, tỉ lệ thời gian chờ; với fix/hotfix thêm ROUND trung bình, tỉ lệ sửa diện rộng và tỉ lệ có regression test. Pane không hiện token (JSON của `zenify track stats --json` vẫn giữ) |
| **Backlog** | Việc chưa làm, nhóm theo nguồn: `manual`, `plane`, `github`, `sdd`, `stale`. Mỗi dòng `<code> · repo · est · title · <số ngày>d`; mục có URL là link bấm được (xem [Backlog](#backlog)). Backlog rỗng hiện `Backlog is empty`; lấy issue GitHub lỗi thì có dòng `⚠ github: <nguyên nhân>` |

Số trên Estimate và Quality chỉ hiện khi đủ mẫu: mỗi loại cần ít nhất 5 việc thật. Chưa đủ thì dòng đó hiện mờ `need 5 samples (have N)`. Tỉ lệ thời gian theo giai đoạn cũng chỉ hiện khi loại đó đủ 5 mẫu. Số từ 10000 trở lên viết gọn `12.3k`, `1.2M`.

Chỉ tab Estimate có chân trang: thang S/M/L hiện hành (đo bằng wall time; nguồn thang `learned` hoặc `default` hiện cạnh số việc, dạng `scale (default, 0 items): S<15m M<45m L<120m`) và các quy tắc hiệu chỉnh, ví dụ gợi ý `raise est 1 size` hoặc `lower est 1 size`. Các tab khác không có chân trang.

## Việc làm dở và ngày làm việc cuối

Đầu tab `Kitchen` có phần `📌 Carried over`, để hôm qua dừng ở đâu thì hôm nay làm tiếp từ đó:

- Dòng `📆 Fri 10/31: 3 done · 1 cancelled · 2 PRs · 2 left open` tóm tắt ngày làm việc cuối trước hôm nay (ngày làm việc, nên sáng thứ Hai là tóm tắt thứ Sáu). Mục bằng 0 thì ẩn.
- Mỗi việc đã bắt đầu từ trước hôm nay mà chưa kết thúc (đang chạy hoặc `stale`) có hai dòng: `- ⏳ 2d <worktree hoặc brief> · <kind> · <phase>` (việc stale dùng `🕸`; tuổi là `Nm`, `Nh` hoặc `Nd` theo wall time), rồi dòng mờ `<tên tab hoặc 8 ký tự session> · <id>`.
- Không có việc dở và không có ngày làm việc cuối thì không có phần này. Khi có việc dở và tab không có việc mở, status line nhắc bằng tip `N items carried over`.

## Chốt sổ hằng ngày

Mỗi ngày một lần, ở session đầu tiên của ngày, SessionStart tự chạy `zenify track close-day`, nhưng chỉ khi workspace đã onboard (sổ có dòng `onboard memory`). Lệnh này cũng chạy tay được bất cứ lúc nào.

- Trong SessionStart, báo cáo đọc trạng thái PR đã cache trên máy (`merged.json`, `pr-status.json`), không gọi `gh`, nên session mở nhanh; khi còn PR đang chờ, báo cáo có dòng `note: PR status from cache`. Chạy tay `zenify track close-day` (khi ngày chưa chốt) thì lệnh hỏi lại `gh` để lấy trạng thái PR mới nhất trước khi viết báo cáo.
- Báo cáo ngày được ghi vào `.zenify/track/days/<D>.md` (tiêu đề `# Track day close: <D>`) và in ra đầu session để agent tóm tắt cho bạn: việc đã xong, PR đã merge, việc làm dở, PR đang chờ kèm bước tiếp theo.
- Sổ `log.md` chỉ giữ 14 ngày gần nhất. Dòng cũ hơn được chuyển sang `.zenify/track/archive/<YYYY-MM>.md` (dòng `history` luôn được chuyển, dòng `plan` được chuyển khi quá 14 ngày); log session và báo cáo ngày cũ nằm ở `archive/sessions/` và `archive/days/`. Marker `.zenify/track/day.closed` ghi nhớ ngày đã chốt.
- Luôn được giữ lại, dù quá 14 ngày: việc đang mở, backlog đang mở, việc có PR merge trong 14 ngày gần nhất, việc mà một dòng `plan` còn giữ nhắc tới (theo id hoặc bid), và các dòng mà cửa sổ stats cần (60 việc đã đo gần nhất, 30 dòng mỗi kind).
- PR trong sổ chưa có trạng thái nào trong cache (SessionStart không hỏi `gh`) vẫn được liệt kê, với bước `status unknown`.
- Mọi lệnh ghi vào `log.md` đều lấy `log.lock`; nếu đang bị giữ thì chờ tối đa 2 giây.

## Kitchen đồng bộ thế nào

Tab `Kitchen` đọc cùng số liệu `zenify track stats --json` như các tab khác, thêm hai phần lấy từ trạng thái sống của từng tab Claude Code.

- Mỗi session có file `<workspace>/.zenify/track/live/<session>.json`, ghi trạng thái `ask`, `permission`, `busy` hoặc `idle`, cùng file `.beat` cạnh nó. Plugin ghi lại `.beat` mỗi 10 giây suốt session khi track bật; `.beat` quá 30 giây thì file bị coi là hết hạn và tab đó rời `Rolling boil` và `Cooking`.
- Plugin chạm file `<workspace>/.zenify/track/pulse` mỗi khi trạng thái sống của một tab đổi và mỗi khi nó ghi một dòng vào session log của mình. Pane theo dõi `pulse` (cùng với sổ và log session) nên cập nhật ngay, và chạy lại số liệu mỗi 10 giây khi còn tab trong `Rolling boil`, mỗi 30 giây khi chỉ còn tab trong `Cooking`, để thời gian chờ chạy và tab đã tắt rời khỏi hai phần đó.
- `Rolling boil` chỉ gồm những tab đang thật sự chờ bạn làm gì đó: tab đang hỏi bạn bằng `AskUserQuestion` và tab đang hiện hộp xin quyền cho bạn bấm. Lời xin quyền mà classifier của Auto mode tự duyệt không hiện hộp nào nên không vào `Rolling boil` và không tính là thời gian chờ. Lời gọi tool bình thường không ghi `live/` và không chạm `pulse`; chỉ prompt (`busy`) và lúc main loop kết thúc một lượt (`idle`) mới đổi trạng thái; tab còn subagent đang chạy (kể cả subagent chạy nền sau khi lượt chính đã xong) vẫn là `busy` cho tới khi subagent cuối cùng dừng (nếu Claude Code không báo subagent dừng, ví dụ tiến trình bị kill, tab còn ở `Cooking` cho tới khi Claude Code khởi động lại).
- `🥘 Cooking` nằm giữa `📌 Carried over` và `🔥 Rolling boil`: các tab đang làm việc ngay lúc này (state `busy`, `.beat` còn sống), tab làm lâu nhất lên đầu. Mỗi tab hai dòng: `- <spinner> <Ns|Nm|Nh> <tab hoặc brief>[ · <kind> · <phase>]` rồi dòng mờ `<8 ký tự session>[ · <id>]`. Spinner và số giây nhảy mỗi 1 giây (chỉ vẽ lại pane, không chạy lệnh nào; việc dò file và chạy lại số liệu vẫn theo nhịp 2 giây), mỗi dòng lệch một khung. Tab ngồi chờ prompt (`idle`) không hiện; không có tab nào thì hiện `no tab is cooking`. Plugin cũ chưa ghi `idle` thì tab chờ prompt vẫn hiện là Cooking cho tới khi plugin của tab đó được cập nhật.
- Mỗi mục trong `Rolling boil` có hai dòng: dòng đầu là icon (❓ đang hỏi, 🔐 chờ quyền), thời gian chờ rồi tên tab Claude Code của session đó (đúng chữ hiện trên tab terminal, lấy từ tiêu đề trong transcript; không có thì dùng `<brief>`); dòng dưới (mờ) là trạng thái (đang hỏi hoặc chờ quyền), `<brief>` và 8 ký tự đầu của session id.
- `<brief>` chọn theo thứ tự: tiêu đề plan, tiêu đề mục backlog, tiêu đề PR, cuối cùng là `<kind> <repo>`.
- `zenify track end --ledger <ledger file>` đếm các dòng `Task <N>: complete` và lưu đường dẫn plan ở dòng đầu của ledger (`# SDD ledger — plan: <path>`) vào dòng `end` (bốn trường) để `Kitchen` lấy được tiêu đề plan. Đường dẫn plan chứa ` · ` hoặc xuống dòng thì dòng `end` giữ ba trường và lệnh cảnh báo trên stderr. File ledger không đọc được thì lệnh cảnh báo trên stderr và vẫn ghi dòng `end` ba trường với 0 task. Đường dẫn plan được lưu nguyên văn; đường dẫn tương đối được đọc tương đối với workspace.

## Thời gian wall và thời gian agent

Thời gian wall là đồng hồ thật từ lúc việc bắt đầu; status line hiện nó đầu tiên (`<wall>m/<est>`). Thời gian agent (`agent <n>m`) là phần Claude thực sự làm. Phần còn lại là thời gian chờ: chờ trả lời câu hỏi (`ask`), chờ cấp quyền (`permission`) hoặc phiên để rảnh (`idle`), hiện ở status line là `wait <n>m`. Est được so với thời gian wall, cùng thang S/M/L. Thời gian agent được tách theo giai đoạn của task (tab Estimate); token vẫn được ghi trong JSON nhưng pane không hiện.

## Chất lượng ship, fix và CI

Mỗi lần `znf:ship` kết thúc, kit ghi kết quả (shipped hoặc blocked, bước bị chặn đầu tiên: lint, build, contract, behaviour, review), số finding theo mức và số vòng fix. `znf:fix` ghi số ROUND, đường sửa hẹp hay rộng và có regression test hay không. Khi tính năng bật, ship mở PR xong sẽ tự gọi `znf:ci-watch` cho từng repo, và kết quả CI (green hoặc red) của mỗi commit được ghi lại, nên tab Quality có tỉ lệ CI đỏ ngay lần đầu.

## Daily plan

Plan là danh sách việc bạn chốt cho hôm nay. Nói "hey claude plan today" (hoặc "plan hôm nay", "daily plan"); agent làm theo `_shared/track.md`:

1. Chạy `zenify track stats --brief --force`.
2. Gom ứng viên theo thứ tự: việc `carried`, PR `prOpen` chưa sẵn sàng merge (`ready == false`), rồi `backlog` (S, M, L, `-`), ước lượng theo `scale`.
3. Lấy lần lượt và dừng ở mục đầu tiên làm tổng est vượt 360 phút (mục đó không lấy, trừ khi danh sách còn rỗng).
4. Đưa danh sách cho bạn duyệt. Chỉ sau khi bạn đồng ý agent mới chạy `zenify track plan set`.

Việc đang chạy ở một tab Claude Code khác còn mở (tín hiệu sống trong 30 giây) không vào plan. Agent in mục "Đang ở tab khác" kèm tên tab để bạn sang đó làm, rồi plan phần còn lại. Tab đã đóng thì việc vẫn là `carried`. Session hiện tại lấy từ `CLAUDE_CODE_SESSION_ID`; Codex không có biến này nên mọi việc đang chạy bị coi là ở tab khác.

```bash
zenify track plan set b-261105-ab12 https://github.com/org/repo/pull/3 cook-261105-cd34
```

- Id nhận được: `bid` của backlog, URL issue hoặc PR, id `sdd` (`repos/<...>/progress.md:<dòng>`), id track (`cook|fix|hotfix|task-YYMMDD-xxxx`). Id sai định dạng hoặc không có id nào thì lệnh thoát khác 0 và không ghi; id trùng trong một lệnh chỉ giữ một.
- Mỗi dòng Plan bắt đầu bằng mã ngắn, ví dụ `- ☐ b81e · chatting · S · …` (xem [Mã ngắn](#mã-ngắn)).
- Sửa plan mà không phải gõ lại cả danh sách: `zenify track plan add <mã>…` thêm vào plan hôm nay, `zenify track plan rm <mã>…` bỏ ra. Mã không khớp mục nào (hoặc `rm` mục không có trong plan) thì lệnh thoát khác 0 và không ghi. Bỏ hết mục thì pane không còn khối Plan.
- Plan lại = chạy `plan set` lại với cả danh sách; dòng `plan` mới nhất của hôm nay thắng, plan của ngày khác bị bỏ qua.
- Mỗi mục có trạng thái `todo`, `running`, `done` hoặc `cancelled`, lấy từ track item tương ứng; mục chỉ là URL PR thì `done` khi PR đã merge.
- Trên tab `Kitchen`, phần `🗓 Plan` liệt kê `- ☐|▶|✅|⛔ <code> · <repo> · <est> · <title>` (có link nếu có URL) rồi dòng mờ `est <x> · used <y>`: tổng mốc của thang S/M/L hiện hành và tổng wall time của các việc trong plan (dưới 60 phút viết `Nm`, còn lại `3h` hoặc `4h30m`).
- Chưa có plan hôm nay thì status line nhắc bằng tip `no plan today`.

## PR cần hành động

Tab `Today` liệt kê mỗi PR đã ghi vào track (bất kỳ ngày nào) hoặc nằm trong plan hôm nay, chưa merge và chưa đóng (tối đa 10, mới nhất trước) một dòng link `🔗 <repo>#<n> · <tuổi> · CI ✅|❌|⏳|- · waiting|changes requested|approved · N threads`. PR sạch (mergeable và không còn thread chưa resolve) hiện `🔗 <repo>#<n> · <tuổi> · ready to merge`. Trạng thái lấy bằng `gh api graphql`, một lần mỗi PR, cùng nhịp 5 phút với việc tra PR đã merge; lỗi thì có dòng `⚠ pr status: <nguyên nhân>`.

## Sổ chung giữa các tab Claude Code

Mọi tab Claude Code trong cùng workspace ghi vào một sổ chung, nên tab `Today` thấy việc của tất cả các tab, còn tab `Kitchen` cho thấy các việc đã xong hôm nay và các tab đang chờ bạn. Mỗi session có file riêng để ghi thời gian và token, nên hai tab chạy song song không đè lên nhau.

File nằm trong workspace:

- `<workspace>/.zenify/track/log.md`: sổ chung, mỗi lượt có dòng `start`, `est`, `end` và các dòng kết quả ship, fix, CI, và dòng `plan` (`<ts> · plan · <ngày> · <id> · <id> …`) do `zenify track plan set`, `plan add` và `plan rm` ghi; dòng `plan` chỉ có ngày (không có id) nghĩa là plan rỗng.
- `<workspace>/.zenify/track/sessions/<session-id>.log`: số liệu theo từng session (gắn id với session, lượt chạy, giai đoạn).

## Est do ai ghi

Est (S, M hoặc L) do `znf:cook`, `znf:fix` và `znf:hotfix` ghi theo hướng dẫn trong `_shared/track.md`: cook ghi sau khi design được duyệt, fix ghi sau khi nguyên nhân được xác nhận, hotfix ghi ngay lúc bắt đầu. Ghi est muộn hơn thì dòng `est` sau cùng thắng.

## Backlog

Tab `Backlog` gom việc chưa làm từ năm nguồn:

- `manual`: việc bạn hoặc agent thêm bằng `zenify track backlog add`.
- `plane`: work item Plane đang mở (state backlog, unstarted, started) giao cho bạn, lấy bằng PAT của chính bạn trong `.zenify/.env` và URL Plane trong manifest. Không có PAT hoặc manifest không khai báo Plane thì nguồn bị bỏ qua; lấy lỗi thì tab báo `⚠ plane: ...`. Tạo việc mới giao cho mình bằng `zenify plane create "<title>" [--project ZEN] [--description <text>]`, lệnh in `plane: ZEN-308 <url>`; thiếu PAT thì báo chạy `zenify up --apply`. Trong Claude Code: `/znf:track add <mô tả> --plane`.
- `github`: issue đang mở trên GitHub mà bạn được giao hoặc do bạn tạo. Kit tìm theo owner (org) lấy từ remote `origin` của từng repo trong `repos/`. Repo không có `origin` và thư mục ẩn (ví dụ `repos/.claude`) được bỏ qua; thư mục khác không phải repo git thì tab báo `⚠ github: ...`.
- `sdd`: mục `minor (deferred)` và `parked` trong `progress.md` của các plan.
- `stale`: việc đã bắt đầu nhưng bị bỏ dở.

Việc `plane` có mã `ZEN-288`. Thứ tự nguồn khi agent lấy việc tiếp theo: danh sách `backlog` trước. Chỉ khi nó rỗng hoặc `zenify track stats --brief` báo `ghError`, agent mới chạy `gh search issues --assignee @me --state open` và `gh search prs --review-requested @me --state open`, đưa kết quả cho bạn chọn rồi start bằng `--from <url>`. Digest đầu session nhắc agent chạy `zenify track stats --brief` trước khi chọn việc. `--brief` là JSON gọn cho agent (việc dở, việc đang ở tab khác `elsewhere`, plan, PR mở, backlog với tiêu đề cắt 80 ký tự, thang S/M/L), không có thời gian, token hay số liệu chất lượng; trên workspace thật nó nhỏ hơn `--json` khoảng 3 lần (7 KB so với 22 KB với 31 mục backlog). Pane vẫn dùng `--json`.

Mỗi dòng Backlog cũng bắt đầu bằng mã ngắn (`b81e` cho việc thêm tay, `#581` cho issue GitHub). Đây là mã bạn gõ vào prompt hoặc vào `/znf:track`.

Mục có `ref` hoặc `id` là URL `http…` hiện thành dòng link: chữ bấm được là `<code> · repo · est · title · <số ngày>d`, trỏ tới URL đó. Dòng là link thì dù bị cắt vẫn bấm được.

Mở tab `Backlog` thì pane chạy lại số liệu và bỏ qua cache backlog 5 phút (`zenify track stats --json --force`), nên issue GitHub mới hiện ngay. Các lần chạy khác dùng cache. Lấy GitHub lỗi (ví dụ chưa `gh auth login`) thì tab báo `⚠ github: ...`, các nguồn còn lại vẫn hiện.

```bash
zenify track backlog add "sửa lỗi export" --repo contact-center-hub --est S --ref https://github.com/org/repo/issues/12
zenify track backlog drop b-261030-ab12
zenify track start --kind fix --repo kit --est S --from b-261030-ab12
```

- `backlog add "<title>" --repo <r> [--est S|M|L] [--ref <url|memory:<name>>]` thêm việc và in id `b-YYMMDD-xxxx`.
- `backlog drop <bid|mã ngắn|sdd id>` bỏ việc khỏi backlog (issue GitHub cũng bỏ được: bằng `#581` hoặc URL); id `sdd` có dạng `repos/<...>/progress.md:<dòng>`.
- `track start --from <bid|mã ngắn|issue URL|sdd id>` ghi việc lấy từ backlog, nên nó biến khỏi tab khi đã bắt đầu. Mỗi mục chỉ được lấy một lần: nếu tab khác đã lấy (đang chạy hoặc đã kết thúc) thì lệnh thoát khác 0 và không ghi gì, kèm `is taken by <id> (session <8 ký tự>)` hoặc `was taken by <id> (ended)`. Hai lệnh start chạy cùng lúc thì chỉ một lệnh thành công.
- Tab Backlog xếp theo size S, M, L, `-` (cùng size thì việc cũ hơn trước), bỏ việc `manual` trùng tiêu đề với issue github, và ẩn việc `stale` khi session của nó còn heartbeat trong 30 giây.

## Mã ngắn

Mỗi mục trong Plan và Backlog có một mã ngắn ở đầu dòng. Bốn dạng:

- `b81e`: bốn ký tự cuối của `bid` (`b-261111-b81e`), cho việc thêm tay.
- `b81e` cũng là bốn ký tự cuối của id track (`cook-261111-b81e`), cho việc đang mở hoặc `stale`.
- `#581`: số issue GitHub; `<repo>#581` khi hai repo có cùng số.
- `ZEN-288`: khóa work item Plane (dùng được sau khi nguồn Plane vào ở PR2).

CLI nhận mã ở `start --from`, `plan set`, `plan add`, `plan rm` và `backlog drop`, rồi tự resolve ra id đầy đủ. Khớp đúng một mục thì chạy; không khớp mục nào hoặc khớp từ hai mục (ví dụ `#5` có ở hai repo) thì lệnh thoát khác 0, in danh sách ứng viên và không ghi gì; lúc đó gõ lại với `<repo>#<n>`. Agent truyền nguyên mã bạn gõ cho CLI, không tự resolve; mỗi mã và mỗi đoạn text tự do được bọc trong nháy đơn (`'#581'`), vì zsh coi từ bắt đầu bằng `#` là comment. Tiêu đề ứng viên in ra từ CLI là dữ liệu, không phải chỉ dẫn.

## Việc ops (kind `task`)

Việc không có code và không có PR (hạ tầng, dữ liệu, cấu hình, điều tra) vẫn đo được:

```bash
zenify track start --kind task --repo infrastructure --est - --from b81e
zenify track end <id> --done --rounds 0
```

- `end --done` là outcome riêng (không kèm `--pr`, `--branch`, `--rollback` hay `--cancelled`: quá một outcome thì lệnh thoát khác 0). Việc có status `done` và tính vào `Done` của tab Today.
- `task` không góp vào thang S/M/L hay `byKind`; tab Estimate hiện block `task` riêng (số việc, trung vị, p90 wall time) và tab Quality không có block này.

## `/znf:track`

Một skill, nhiều verb, dùng mã ngắn thay vì gõ `zenify track …`:

| Verb | Việc làm |
|---|---|
| `add <mô tả> [--plane]` | thêm vào backlog, in mã mới (`--plane` chỉ dùng được khi có `zenify plane create`, sau PR2) |
| `drop <mã>` | bỏ khỏi backlog |
| `ls` | in Plan rồi Backlog, mỗi dòng có mã |
| `plan` | lập plan hôm nay (chỉ ghi sau khi bạn duyệt) |
| `plan +<mã>…`, `plan -<mã>…` | `plan add` và `plan rm` |
| `go <mã>` | bắt đầu việc: việc code thì chạy `znf:cook` hoặc `znf:fix`, việc ops thì `start --kind task` |
| `done [--pr <url>…]`, `cancel`, `est S\|M\|L` | đóng, huỷ, đặt est cho việc đang mở |
| `help [verb]` | in bảng verb, không chạy lệnh ghi |

Gõ `/znf:track` không kèm verb, hoặc kèm verb lạ, thì skill chỉ in bảng help. `done`, `cancel`, `est` dùng id session đã nhận lúc `start`; mất id thì skill đọc mảng `running` của `zenify track stats --brief`: đúng một thì dùng, từ hai thì hỏi bạn, không có thì báo. CLI báo lỗi (mã trùng, `taken by`) thì skill đưa nguyên thông báo cho bạn chọn, không tự thử lại.

## Onboard

Workspace mới chưa có lịch sử nên thang S/M/L dùng mặc định. Hai lệnh nạp dữ liệu ban đầu:

```bash
zenify track onboard memory
zenify track onboard history            # xem trước, chỉ đọc
zenify track onboard history --write --since 2026-09-01
```

- `onboard memory` đánh dấu workspace đã onboard từ memory của agent.
- `onboard history` quét transcript Claude Code (`~/.claude/projects`), gom theo repo và worktree rồi in bảng xem trước. Thêm `--write` mới ghi dòng `history` vào sổ; chạy lại không tạo dòng trùng. `--since YYYY-MM-DD` giới hạn từ ngày đó.

Thang mặc định là S<15m, M<45m, L<120m. Từ 20 việc thật đã xong có est, thang chuyển sang `learned` theo wall time. Dòng `history` không góp vào thang, Estimate, chất lượng hay số hôm nay.

Xem [`zenify track`](/reference/cli/zenify_track), trong đó `stats --force` bỏ qua cache backlog.

## Settings

Các thiết lập của kit là các dòng của plugin `znf` trong `/config` của Claude Code (key `znf.<field>`). Đổi một dòng thì mod chạy đúng một lệnh `zenify`, không tốn lượt chat nào; lệnh lỗi thì `/config` từ chối thay đổi, hiện stderr của lệnh và dòng giữ giá trị cũ. Nguồn sự thật vẫn là overlay của workspace và `env.ZNF_REVIEWER`: mỗi lần mở session, mod đọc [`zenify feature`](/reference/cli/zenify_feature) và [`zenify reviewer get`](/reference/cli/zenify_reviewer_get) rồi kéo dòng nào lệch về đúng nguồn; đọc lỗi thì hiện toast `⚠` và không sửa dòng.

| Dòng | Giá trị | Tác dụng |
|---|---|---|
| **Dev track** (`znf.track`) | bật/tắt, mặc định tắt | Chạy `zenify feature track on\|off` cho workspace hiện tại |
| **GitHub issues** (`znf.githubIssues`) | bật/tắt, mặc định tắt | Chạy `zenify feature github.issues on\|off` |
| **Track pane** (`znf.trackPane`) | `auto` (mặc định), `command`, `off` | `auto`: pane tự mở khi bạn gõ cook/fix/hotfix/ship, trừ khi bạn đã đóng nó; `command`: chỉ mở bằng `/track-pane`; `off`: không mở và không có lệnh `/track-pane`. Status line không đổi theo dòng này |
| **Status line tips** (`znf.tips`) | bật/tắt, mặc định bật | Tắt thì status line không còn tip theo ngữ cảnh (Backlog, estimate, vượt est, PR chờ merge, PR CI đỏ, thread chưa resolve, bị yêu cầu sửa, sẵn sàng merge, việc mở từ 5 ngày). Tip onboard và câu bật track khi track off vẫn hiện. Chỉ là tùy chọn của plugin, không chạy lệnh nào |
| **Reviewer** (`znf.reviewer`) | `agent` (mặc định), `codex` | Chạy [`zenify reviewer set`](/reference/cli/zenify_reviewer_set); có hiệu lực ở session mới |

Env của kit không nằm trong `/config`: xem bằng [`zenify env list`](/reference/cli/zenify_env_list) và sửa bằng `zenify env set KEY` trong terminal (nhập ẩn).
