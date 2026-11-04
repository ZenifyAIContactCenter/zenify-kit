---
title: "Pane track: đo thời gian, token và chất lượng của từng lượt làm việc"
---

# Pane track

`track` ghi lại mỗi lượt `znf:cook`, `znf:fix`, `znf:hotfix` và `znf:ship` để bạn thấy việc tốn bao lâu, tốn bao nhiêu token và ship có suôn sẻ không. Số liệu hiện trong một pane bên cạnh chat và một status line ở đáy, tự cập nhật, không tốn lượt chat nào. Tính năng mặc định tắt.

## Bật và tắt

```bash
zenify feature track on
zenify feature track off
```

Ngay trong Claude Code, gõ `/config` rồi bật dòng `Dev track` của plugin `znf` để làm tương tự (xem [Settings](#settings)); status line tự cập nhật trong vài giây. Lệnh áp dụng cho cả workspace. Khi bật, digest đầu session chỉ cho agent đọc `_shared/track.md`; agent gọi các lệnh [`zenify track`](/reference/cli/zenify_track) để ghi số liệu, bạn không cần gõ gì. Tắt thì mọi lệnh ghi của `zenify track` in `track: off` và không ghi.

Pane là một mod trong plugin `znf`, có sẵn sau `zenify skills sync`. Gõ `/track-pane` để mở hoặc đóng pane. Pane cũng tự mở khi prompt của bạn là `cook`, `fix`, `hotfix` hoặc `ship` (có hoặc không có tiền tố `znf:`), nếu dòng `Track pane` trong `/config` là `auto` (mặc định). Pane, lệnh, các dòng `znf` trong `/config` và status line chỉ có khi Claude Code mở trong một zenify workspace (có `.zenify/manifest.json` ở thư mục hiện tại hoặc thư mục cha); ngoài workspace chúng ẩn hết.

## Status line

Status line luôn hiện, kể cả khi pane đóng. Toàn bộ chữ trên pane và status line là tiếng Anh.

- Khi tắt: `⏸ track off · enable: /config → znf`
- Khi bật và tab này đang có việc mở: `track ▶ <id> · <phase> · <wall>m/<est> (agent <n>m) · wait <n>m · Today: done/started · PR n (m merged) · hit n%`
- Khi bật và tab này chưa có việc: `track ⏸ · Today: done/started · ...`
- Khi bật, tab này chưa có việc và workspace chưa onboard: `track ⏸ · Tip: onboard the agent to pick work — type "hey claude onboard track"`
- Số đầu tiên là wall time (đồng hồ thật từ lúc bắt đầu), kèm `<est>` nếu việc có size; `(agent <n>m)` là thời gian agent thật sự làm, chỉ hiện khi lớn hơn 0. Chi tiết ở [Thời gian wall và thời gian agent](#thời-gian-wall-và-thời-gian-agent).
- `hit n%` chỉ hiện khi đã có ít nhất 5 việc có est.
- Tip theo ngữ cảnh: mỗi lúc tối đa một tip, nối cuối dòng bằng ` · `, chọn theo thứ tự ưu tiên (onboard ở trên trước):
  - tab này không có id mở và còn việc làm dở từ các ngày trước: `Tip: N items carried over — type "hey claude continue"`
  - tab này không có id mở và Backlog còn việc: `Tip: N items in Backlog — type "hey claude next task"`
  - có id mở, chưa có estimate và đã chạy từ 5 phút: `Tip: no estimate yet — type "hey claude estimate this"`
  - có id mở, đã vượt ngưỡng của size est: `Tip: over est M (50m/45m) — split the work or re-estimate`
  - còn PR đã mở (bất kỳ ngày nào) chưa merge: `Tip: N PRs waiting to merge — type "hey claude check PR"`

Mục nào bằng 0 hoặc chưa biết thì ẩn, riêng `Today` luôn hiện. Đọc số liệu lỗi thì status line hiện `⚠ track error: <nguyên nhân>`.

## Năm tab

| Tab | Nội dung |
|---|---|
| **Kitchen** | Tab mặc định, giống nhau ở mọi session. Thứ tự từ trên xuống: `📌 Carried over` (xem [Việc làm dở và ngày làm việc cuối](#việc-làm-dở-và-ngày-làm-việc-cuối)), `🥘 Cooking`, `🔥 Rolling boil`, `🍳 Cooked`. `🔥 Rolling boil`: các tab Claude Code đang chờ bạn, mỗi dòng `- <❓ hoặc 🔐> <thời gian chờ> <tab>` (`45s`, `2m`, `2h`, làm tròn xuống); không có thì hiện `no tab is waiting`. `🍳 Cooked`: các việc đã xong hôm nay, mỗi dòng `- <icon> <HH:MM> <brief>` (giờ máy; icon cho biết kết quả: ✅ done và mọi PR đã merge, ⏳ done nhưng còn PR chưa merge, 🚀 pushed, ⛔ cancelled, ↩️ rollback), mỗi PR một dòng `  🔗 <repo>#<n>` là link bấm được tới PR; chưa có thì hiện `nothing finished today`. Lấy tiêu đề lỗi thì có dòng `⚠ title error: <nguyên nhân>` |
| **Today** | Hôm nay của cả workspace: `Done`, `started`, `Cancelled`, `Rollback` (mục bằng 0 thì ẩn), `Time <wall>m · agent <n>m`, `SDD tasks` (chỉ khi hôm nay có cook xong), `Rounds`, `PR n (m merged)`, `Plans written today` theo repo; danh sách `Running` gom theo session, tab hiện tại ghi `(this tab)`; việc bị bỏ dở ghi `stale` |
| **Estimate** | Với mỗi loại (cook, fix, hotfix): est đúng/thấp/cao so với thực tế (`hit`, `under`, `over`); hotfix hiện số việc, trung vị và p90; `agent time by phase` là tỉ lệ thời gian agent theo giai đoạn |
| **Quality** | Với mỗi loại: tỉ lệ ship bị chặn và bước chặn nhiều nhất, finding C/H/M/L trung bình, số vòng fix loop, tỉ lệ CI đỏ lần đầu, tỉ lệ thời gian chờ; với fix/hotfix thêm ROUND trung bình, tỉ lệ sửa diện rộng và tỉ lệ có regression test. Pane không hiện token (JSON của `zenify track stats --json` vẫn giữ) |
| **Backlog** | Việc chưa làm, nhóm theo nguồn: `manual`, `github`, `sdd`, `stale`. Mỗi dòng `repo · est · title · <số ngày>d`; mục có URL là link bấm được (xem [Backlog](#backlog)). Backlog rỗng hiện `Backlog is empty`; lấy issue GitHub lỗi thì có dòng `⚠ github: <nguyên nhân>` |

Số trên Estimate và Quality chỉ hiện khi đủ mẫu: mỗi loại cần ít nhất 5 việc thật. Chưa đủ thì dòng đó hiện mờ `need 5 samples (have N)`. Tỉ lệ thời gian theo giai đoạn cũng chỉ hiện khi loại đó đủ 5 mẫu. Số từ 10000 trở lên viết gọn `12.3k`, `1.2M`.

Chỉ tab Estimate có chân trang: thang S/M/L hiện hành (đo bằng wall time; nguồn thang `learned` hoặc `default` hiện cạnh số việc, dạng `scale (default, 0 items): S<15m M<45m L<120m`) và các quy tắc hiệu chỉnh, ví dụ gợi ý `raise est 1 size` hoặc `lower est 1 size`. Các tab khác không có chân trang.

## Việc làm dở và ngày làm việc cuối

Đầu tab `Kitchen` có phần `📌 Carried over`, để hôm qua dừng ở đâu thì hôm nay làm tiếp từ đó:

- Dòng `📆 Fri 10/31: 3 done · 1 cancelled · 2 PRs · 2 left open` tóm tắt ngày làm việc cuối trước hôm nay (ngày làm việc, nên sáng thứ Hai là tóm tắt thứ Sáu). Mục bằng 0 thì ẩn.
- Mỗi việc đã bắt đầu từ trước hôm nay mà chưa kết thúc (đang chạy hoặc `stale`) có hai dòng: `- ⏳ 2d <worktree hoặc brief> · <kind> · <phase>` (việc stale dùng `🕸`; tuổi là `Nm`, `Nh` hoặc `Nd` theo wall time), rồi dòng mờ `<tên tab hoặc 8 ký tự session> · <id>`.
- Không có việc dở và không có ngày làm việc cuối thì không có phần này. Khi có việc dở và tab không có việc mở, status line nhắc bằng tip `N items carried over`.

## Kitchen đồng bộ thế nào

Tab `Kitchen` đọc cùng số liệu `zenify track stats --json` như các tab khác, thêm hai phần lấy từ trạng thái sống của từng tab Claude Code.

- Mỗi session có file `<workspace>/.zenify/track/live/<session>.json`, ghi trạng thái `ask`, `permission`, `busy` hoặc `idle`, cùng file `.beat` cạnh nó. Plugin ghi lại `.beat` mỗi 10 giây suốt session khi track bật; `.beat` quá 30 giây thì file bị coi là hết hạn và tab đó rời `Rolling boil` và `Cooking`.
- Plugin chạm file `<workspace>/.zenify/track/pulse` mỗi khi trạng thái sống của một tab đổi và mỗi khi nó ghi một dòng vào session log của mình. Pane theo dõi `pulse` (cùng với sổ và log session) nên cập nhật ngay, và chạy lại số liệu mỗi 10 giây khi còn tab trong `Rolling boil`, mỗi 30 giây khi chỉ còn tab trong `Cooking`, để thời gian chờ chạy và tab đã tắt rời khỏi hai phần đó.
- `Rolling boil` chỉ gồm những tab đang thật sự chờ bạn làm gì đó: tab đang hỏi bạn bằng `AskUserQuestion` và tab đang hiện hộp xin quyền cho bạn bấm. Lời xin quyền mà classifier của Auto mode tự duyệt không hiện hộp nào nên không vào `Rolling boil` và không tính là thời gian chờ. Lời gọi tool bình thường không ghi `live/` và không chạm `pulse`; chỉ prompt (`busy`) và lúc main loop kết thúc một lượt (`idle`) mới đổi trạng thái; tab còn subagent đang chạy (kể cả subagent chạy nền sau khi lượt chính đã xong) vẫn là `busy` cho tới khi subagent cuối cùng dừng (nếu Claude Code không báo subagent dừng, ví dụ tiến trình bị kill, tab còn ở `Cooking` cho tới khi Claude Code khởi động lại).
- `🥘 Cooking` nằm giữa `📌 Carried over` và `🔥 Rolling boil`: các tab đang làm việc ngay lúc này (state `busy`, `.beat` còn sống), tab làm lâu nhất lên đầu. Mỗi tab hai dòng: `- <spinner> <Ns|Nm|Nh> <tab hoặc brief>[ · <kind> · <phase>]` rồi dòng mờ `<8 ký tự session>[ · <id>]`. Spinner quay theo tick 2 giây, mỗi dòng lệch một khung. Tab ngồi chờ prompt (`idle`) không hiện; không có tab nào thì hiện `no tab is cooking`. Plugin cũ chưa ghi `idle` thì tab chờ prompt vẫn hiện là Cooking cho tới khi plugin của tab đó được cập nhật.
- Mỗi mục trong `Rolling boil` có hai dòng: dòng đầu là icon (❓ đang hỏi, 🔐 chờ quyền), thời gian chờ rồi tên tab Claude Code của session đó (đúng chữ hiện trên tab terminal, lấy từ tiêu đề trong transcript; không có thì dùng `<brief>`); dòng dưới (mờ) là trạng thái (đang hỏi hoặc chờ quyền), `<brief>` và 8 ký tự đầu của session id.
- `<brief>` chọn theo thứ tự: tiêu đề plan, tiêu đề mục backlog, tiêu đề PR, cuối cùng là `<kind> <repo>`.
- `zenify track end --ledger <ledger file>` đếm các dòng `Task <N>: complete` và lưu đường dẫn plan ở dòng đầu của ledger (`# SDD ledger — plan: <path>`) vào dòng `end` (bốn trường) để `Kitchen` lấy được tiêu đề plan. Đường dẫn plan chứa ` · ` hoặc xuống dòng thì dòng `end` giữ ba trường và lệnh cảnh báo trên stderr. File ledger không đọc được thì lệnh cảnh báo trên stderr và vẫn ghi dòng `end` ba trường với 0 task. Đường dẫn plan được lưu nguyên văn; đường dẫn tương đối được đọc tương đối với workspace.

## Thời gian wall và thời gian agent

Thời gian wall là đồng hồ thật từ lúc việc bắt đầu; status line hiện nó đầu tiên (`<wall>m/<est>`). Thời gian agent (`agent <n>m`) là phần Claude thực sự làm. Phần còn lại là thời gian chờ: chờ trả lời câu hỏi (`ask`), chờ cấp quyền (`permission`) hoặc phiên để rảnh (`idle`), hiện ở status line là `wait <n>m`. Est được so với thời gian wall, cùng thang S/M/L. Thời gian agent được tách theo giai đoạn của task (tab Estimate); token vẫn được ghi trong JSON nhưng pane không hiện.

## Chất lượng ship, fix và CI

Mỗi lần `znf:ship` kết thúc, kit ghi kết quả (shipped hoặc blocked, bước bị chặn đầu tiên: lint, build, contract, behaviour, review), số finding theo mức và số vòng fix. `znf:fix` ghi số ROUND, đường sửa hẹp hay rộng và có regression test hay không. Khi tính năng bật, ship mở PR xong sẽ tự gọi `znf:ci-watch` cho từng repo, và kết quả CI (green hoặc red) của mỗi commit được ghi lại, nên tab Quality có tỉ lệ CI đỏ ngay lần đầu.

## Sổ chung giữa các tab Claude Code

Mọi tab Claude Code trong cùng workspace ghi vào một sổ chung, nên tab `Today` thấy việc của tất cả các tab, còn tab `Kitchen` cho thấy các việc đã xong hôm nay và các tab đang chờ bạn. Mỗi session có file riêng để ghi thời gian và token, nên hai tab chạy song song không đè lên nhau.

File nằm trong workspace:

- `<workspace>/.zenify/track/log.md`: sổ chung, mỗi lượt có dòng `start`, `est`, `end` và các dòng kết quả ship, fix, CI.
- `<workspace>/.zenify/track/sessions/<session-id>.log`: số liệu theo từng session (gắn id với session, lượt chạy, giai đoạn).

## Est do ai ghi

Est (S, M hoặc L) do `znf:cook`, `znf:fix` và `znf:hotfix` ghi theo hướng dẫn trong `_shared/track.md`: cook ghi sau khi design được duyệt, fix ghi sau khi nguyên nhân được xác nhận, hotfix ghi ngay lúc bắt đầu. Ghi est muộn hơn thì dòng `est` sau cùng thắng.

## Backlog

Tab `Backlog` gom việc chưa làm từ bốn nguồn:

- `manual`: việc bạn hoặc agent thêm bằng `zenify track backlog add`.
- `github`: issue đang mở trên GitHub mà bạn được giao hoặc do bạn tạo. Kit tìm theo owner (org) lấy từ remote `origin` của từng repo trong `repos/`. Repo không có `origin` và thư mục ẩn (ví dụ `repos/.claude`) được bỏ qua; thư mục khác không phải repo git thì tab báo `⚠ github: ...`.
- `sdd`: mục `minor (deferred)` và `parked` trong `progress.md` của các plan.
- `stale`: việc đã bắt đầu nhưng bị bỏ dở.

Mục có `ref` hoặc `id` là URL `http…` hiện thành dòng link: chữ bấm được là `repo · est · title · <số ngày>d`, trỏ tới URL đó. Dòng là link thì dù bị cắt vẫn bấm được.

Mở tab `Backlog` thì pane chạy lại số liệu và bỏ qua cache backlog 5 phút (`zenify track stats --json --force`), nên issue GitHub mới hiện ngay. Các lần chạy khác dùng cache. Lấy GitHub lỗi (ví dụ chưa `gh auth login`) thì tab báo `⚠ github: ...`, ba nguồn còn lại vẫn hiện.

```bash
zenify track backlog add "sửa lỗi export" --repo contact-center-hub --est S --ref https://github.com/org/repo/issues/12
zenify track backlog drop b-261030-ab12
zenify track start --kind fix --repo kit --est S --from b-261030-ab12
```

- `backlog add "<title>" --repo <r> [--est S|M|L] [--ref <url|memory:<name>>]` thêm việc và in id `b-YYMMDD-xxxx`.
- `backlog drop <bid|sdd id>` bỏ việc khỏi backlog; id `sdd` có dạng `repos/<...>/progress.md:<dòng>`.
- `track start --from <bid|issue URL|sdd id>` ghi việc lấy từ backlog, nên nó biến khỏi tab khi đã bắt đầu. Mỗi mục chỉ được lấy một lần: nếu tab khác đã lấy (đang chạy hoặc đã kết thúc) thì lệnh thoát khác 0 và không ghi gì, kèm `is taken by <id> (session <8 ký tự>)` hoặc `was taken by <id> (ended)`. Hai lệnh start chạy cùng lúc thì chỉ một lệnh thành công.
- Tab Backlog xếp theo size S, M, L, `-` (cùng size thì việc cũ hơn trước), bỏ việc `manual` trùng tiêu đề với issue github, và ẩn việc `stale` khi session của nó còn heartbeat trong 30 giây.

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
| **Track pane** (`znf.trackPane`) | `auto` (mặc định), `command`, `off` | `auto`: pane tự mở khi cook/fix/hotfix/ship; `command`: chỉ mở bằng `/track-pane`; `off`: không mở và không có lệnh `/track-pane`. Status line không đổi theo dòng này |
| **Status line tips** (`znf.tips`) | bật/tắt, mặc định bật | Tắt thì status line không còn tip theo ngữ cảnh (Backlog, estimate, vượt est, PR chờ merge). Tip onboard và câu bật track khi track off vẫn hiện. Chỉ là tùy chọn của plugin, không chạy lệnh nào |
| **Reviewer** (`znf.reviewer`) | `agent` (mặc định), `codex` | Chạy [`zenify reviewer set`](/reference/cli/zenify_reviewer_set); có hiệu lực ở session mới |

Env của kit không nằm trong `/config`: xem bằng [`zenify env list`](/reference/cli/zenify_env_list) và sửa bằng `zenify env set KEY` trong terminal (nhập ẩn).
