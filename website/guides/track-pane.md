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

Pane là một mod trong plugin `znf`, có sẵn sau `zenify skills sync`. Gõ `/track-pane` để mở hoặc đóng pane. Pane cũng tự mở khi prompt của bạn là `cook`, `fix`, `hotfix` hoặc `ship` (có hoặc không có tiền tố `znf:`), nếu dòng `Track pane` trong `/config` là `auto` (mặc định).

## Status line

Status line luôn hiện, kể cả khi pane đóng.

- Khi tắt: `⏸ track off · bật: /config → znf`
- Khi bật và tab này đang có việc mở: `track ▶ <id> · <giai đoạn> · <phút agent>/<est> · chờ <phút> · Hôm nay: xong/bắt đầu · PR n (m merged) · hit n%`
- Khi bật và tab này chưa có việc: `track ⏸ · Hôm nay: xong/bắt đầu · ...`

Mục nào bằng 0 hoặc chưa biết thì ẩn, riêng `Hôm nay` luôn hiện. Đọc số liệu lỗi thì status line hiện `⚠ track lỗi: <nguyên nhân>`.

## Năm tab

| Tab | Nội dung |
|---|---|
| **Tab này** | Việc đang mở của đúng tab Claude Code này: id, loại, repo, est, giai đoạn hiện tại, thời gian agent so với thang S/M/L, thời gian chờ (hỏi, quyền, rảnh), thời gian agent và chờ theo từng giai đoạn, token theo model, PR đã mở; và các id đã xong hôm nay của tab này |
| **Tổng** | Hôm nay của cả workspace: xong/bắt đầu, số task, số vòng, PR đã mở và đã merge, plan theo repo; danh sách việc đang chạy gom theo session, tab hiện tại ghi `(tab này)`; dòng `stale` cho việc bị bỏ dở |
| **Estimate** | Với mỗi loại (cook, fix, hotfix): est đúng/thấp/cao so với thực tế; hotfix hiện số việc, trung vị và p90; tỉ lệ agent theo giai đoạn |
| **Chất lượng** | Với mỗi loại: tỉ lệ ship bị chặn và bước chặn nhiều nhất, finding C/H/M/L trung bình, số vòng fix loop, tỉ lệ CI đỏ lần đầu, tỉ lệ thời gian chờ, token trung bình theo model; với fix/hotfix thêm ROUND trung bình, tỉ lệ sửa diện rộng và tỉ lệ có regression test |
| **Backlog** | Việc chưa làm, nhóm theo nguồn: `manual`, `github`, `sdd`, `stale`. Mỗi dòng `repo · est · title · <số ngày>d`. Backlog rỗng hiện `Backlog trống`; lấy issue GitHub lỗi thì có dòng `⚠ github: <nguyên nhân>` |

Dưới mọi tab là thang S/M/L hiện hành (lấy từ lịch sử của workspace; nguồn thang `learned`, `history` hoặc `default` hiện cạnh số việc) và các quy tắc hiệu chỉnh, ví dụ gợi ý tăng hoặc giảm est một size.

## Thời gian agent và thời gian chờ

Thời gian được chia hai loại. Thời gian agent là lúc Claude đang làm việc. Thời gian chờ là lúc phiên đứng chờ bạn: chờ trả lời câu hỏi (`hỏi`), chờ cấp quyền (`quyền`) hoặc phiên để rảnh (`rảnh`). Estimate so với thời gian agent, không tính thời gian chờ, nên một task nằm đợi bạn cả buổi không bị tính là lâu. Thời gian cũng được tách theo giai đoạn của task, và token tách theo model.

## Chất lượng ship, fix và CI

Mỗi lần `znf:ship` kết thúc, kit ghi kết quả (shipped hoặc blocked, bước bị chặn đầu tiên: lint, build, contract, behaviour, review), số finding theo mức và số vòng fix. `znf:fix` ghi số ROUND, đường sửa hẹp hay rộng và có regression test hay không. Khi tính năng bật, ship mở PR xong sẽ tự gọi `znf:ci-watch` cho từng repo, và kết quả CI (green hoặc red) của mỗi commit được ghi lại, nên tab Chất lượng có tỉ lệ CI đỏ ngay lần đầu.

## Sổ chung giữa các tab Claude Code

Mọi tab Claude Code trong cùng workspace ghi vào một sổ chung, nên tab `Tổng` thấy việc của tất cả các tab, còn tab `Tab này` chỉ lọc ra phần của session hiện tại. Mỗi session có file riêng để ghi thời gian và token, nên hai tab chạy song song không đè lên nhau.

File nằm trong workspace:

- `<workspace>/.zenify/track/log.md`: sổ chung, mỗi lượt có dòng `start`, `est`, `end` và các dòng kết quả ship, fix, CI.
- `<workspace>/.zenify/track/sessions/<session-id>.log`: số liệu theo từng session (gắn id với session, lượt chạy, giai đoạn).

## Est do ai ghi

Est (S, M hoặc L) do `znf:cook`, `znf:fix` và `znf:hotfix` ghi theo hướng dẫn trong `_shared/track.md`: cook ghi sau khi design được duyệt, fix ghi sau khi nguyên nhân được xác nhận, hotfix ghi ngay lúc bắt đầu. Ghi est muộn hơn thì dòng `est` sau cùng thắng.

## Backlog

Tab `Backlog` gom việc chưa làm từ bốn nguồn:

- `manual`: việc bạn hoặc agent thêm bằng `zenify track backlog add`.
- `github`: issue đang mở trên GitHub mà bạn được giao hoặc do bạn tạo.
- `sdd`: mục `minor (deferred)` và `parked` trong `progress.md` của các plan.
- `stale`: việc đã bắt đầu nhưng bị bỏ dở.

Mở tab `Backlog` thì pane chạy lại số liệu và bỏ qua cache backlog 5 phút (`zenify track stats --json --force`), nên issue GitHub mới hiện ngay. Các lần chạy khác dùng cache. Lấy GitHub lỗi (ví dụ chưa `gh auth login`) thì tab báo `⚠ github: ...`, ba nguồn còn lại vẫn hiện.

```bash
zenify track backlog add "sửa lỗi export" --repo contact-center-hub --est S --ref https://github.com/org/repo/issues/12
zenify track backlog drop b-261030-ab12
zenify track start --kind fix --repo kit --est S --from b-261030-ab12
```

- `backlog add "<title>" --repo <r> [--est S|M|L] [--ref <url|memory:<name>>]` thêm việc và in id `b-YYMMDD-xxxx`.
- `backlog drop <bid|sdd id>` bỏ việc khỏi backlog; id `sdd` có dạng `repos/<...>/progress.md:<dòng>`.
- `track start --from <id|issue URL>` ghi việc lấy từ backlog, nên nó biến khỏi tab khi đã bắt đầu.

## Onboard

Workspace mới chưa có lịch sử nên thang S/M/L dùng mặc định. Hai lệnh nạp dữ liệu ban đầu:

```bash
zenify track onboard memory
zenify track onboard history            # xem trước, chỉ đọc
zenify track onboard history --write --since 2026-09-01
```

- `onboard memory` đánh dấu workspace đã onboard từ memory của agent.
- `onboard history` quét transcript Claude Code (`~/.claude/projects`), gom theo repo và worktree rồi in bảng xem trước. Thêm `--write` mới ghi dòng `history` vào sổ; chạy lại không tạo dòng trùng. `--since YYYY-MM-DD` giới hạn từ ngày đó.

Khi có đủ việc (thật cộng `history`), thang S/M/L chuyển nguồn sang `history`, rồi `learned` khi có từ 20 việc thật đã đo. Dòng `history` chỉ góp vào thang và token trung bình, không vào Estimate, luật hiệu chỉnh hay số hôm nay.

Xem [`zenify track`](/reference/cli/zenify_track), trong đó `stats --force` bỏ qua cache backlog.

## Settings

Các thiết lập của kit là các dòng của plugin `znf` trong `/config` của Claude Code (key `znf.<field>`). Đổi một dòng thì mod chạy đúng một lệnh `zenify`, không tốn lượt chat nào; lệnh lỗi thì `/config` từ chối thay đổi, hiện stderr của lệnh và dòng giữ giá trị cũ. Nguồn sự thật vẫn là overlay của workspace và `env.ZNF_REVIEWER`: mỗi lần mở session, mod đọc [`zenify feature`](/reference/cli/zenify_feature) và [`zenify reviewer`](/reference/cli/zenify_reviewer) rồi kéo dòng nào lệch về đúng nguồn; đọc lỗi thì hiện toast `⚠` và không sửa dòng.

| Dòng | Giá trị | Tác dụng |
|---|---|---|
| **Dev track** (`znf.track`) | bật/tắt, mặc định tắt | Chạy `zenify feature track on\|off` cho workspace hiện tại |
| **GitHub issues** (`znf.githubIssues`) | bật/tắt, mặc định tắt | Chạy `zenify feature github.issues on\|off` |
| **Track pane** (`znf.trackPane`) | `auto` (mặc định), `command`, `off` | `auto`: pane tự mở khi cook/fix/hotfix/ship; `command`: chỉ mở bằng `/track-pane`; `off`: không mở và không có lệnh `/track-pane`. Status line không đổi theo dòng này |
| **Reviewer** (`znf.reviewer`) | `agent` (mặc định), `codex` | Chạy [`zenify reviewer set`](/reference/cli/zenify_reviewer_set); có hiệu lực ở session mới |

Env của kit không nằm trong `/config`: xem bằng [`zenify env list`](/reference/cli/zenify_env_list) và sửa bằng `zenify env set KEY` trong terminal (nhập ẩn).
