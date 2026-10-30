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

Lệnh áp dụng cho cả workspace. Khi bật, digest đầu session chỉ cho agent đọc `_shared/track.md`; agent gọi các lệnh [`zenify track`](/reference/cli/zenify_track) để ghi số liệu, bạn không cần gõ gì. Tắt thì mọi lệnh `zenify track` in `track: off` và không ghi.

Pane là một mod trong plugin `znf`, có sẵn sau `zenify skills sync`. Gõ `/track-pane` để mở hoặc đóng pane. Pane chỉ dock cạnh chat khi Claude Code chạy fullscreen (`"tui": "fullscreen"` trong `~/.claude/settings.json`).

## Status line

Status line luôn hiện, kể cả khi pane đóng.

- Khi tắt: `⏸ znf: track off · bật: zenify feature track on`
- Khi bật và tab này đang có việc mở: `znf: track ▶ <id> · <giai đoạn> · <phút agent>/<est> · chờ <phút> · Hôm nay: xong/bắt đầu · PR n (m merged) · hit n%`
- Khi bật và tab này chưa có việc: `znf: track ⏸ · Hôm nay: xong/bắt đầu · ...`

Mục nào bằng 0 hoặc chưa biết thì ẩn, riêng `Hôm nay` luôn hiện. Đọc số liệu lỗi thì status line hiện `⚠ znf: track lỗi: <nguyên nhân>`.

## Bốn tab

| Tab | Nội dung |
|---|---|
| **Tab này** | Việc đang mở của đúng tab Claude Code này: id, loại, repo, est, giai đoạn hiện tại, thời gian agent so với thang S/M/L, thời gian chờ (hỏi, quyền, rảnh), thời gian agent và chờ theo từng giai đoạn, token theo model, PR đã mở; và các id đã xong hôm nay của tab này |
| **Tổng** | Hôm nay của cả workspace: xong/bắt đầu, số task, số vòng, PR đã mở và đã merge, plan theo repo; danh sách việc đang chạy gom theo session, tab hiện tại ghi `(tab này)`; dòng `stale` cho việc bị bỏ dở |
| **Estimate** | Với mỗi loại (cook, fix, hotfix): est đúng/thấp/cao so với thực tế; hotfix hiện số việc, trung vị và p90; tỉ lệ agent theo giai đoạn |
| **Chất lượng** | Với mỗi loại: tỉ lệ ship bị chặn và bước chặn nhiều nhất, finding C/H/M/L trung bình, số vòng fix loop, tỉ lệ CI đỏ lần đầu, tỉ lệ thời gian chờ, token trung bình theo model; với fix/hotfix thêm ROUND trung bình, tỉ lệ sửa diện rộng và tỉ lệ có regression test |

Dưới mọi tab là thang S/M/L hiện hành (lấy từ lịch sử của workspace) và các quy tắc hiệu chỉnh, ví dụ gợi ý tăng hoặc giảm est một size.

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
