---
title: Ngân sách context
---

# Ngân sách context

## Tổng quan

Chi phí một session Claude Code không nằm ở số chữ model viết ra mà ở khối context được đọc lại mỗi turn. Đo trên một workspace trong 30 ngày (`zenify cost`): 95% token là cache read, context của phiên chính median 186k token mỗi turn, và thứ lấp đầy nó là kết quả tool (45%), trong đó Read chiếm 61%. Mọi kết quả đã vào context nằm lại đó cho đến `/clear`.

Context lớn không chỉ đắt. Các phép đo độc lập (Context Rot, NoLiMa) cho thấy độ chính xác giảm rõ khi context vượt vài chục nghìn token. Giữ context nhỏ là chuyện chất lượng trước, tiền sau.

Kit có bốn cơ chế cho việc này. Ba cái đầu là cơ học, cái cuối là quy tắc.

## read-guard: chặn Read quá lớn

Hook `PreToolUse` trên tool Read. Nó từ chối, kèm gợi ý, khi:

- file text trên 200 KB mà lệnh Read không có cả `offset` và `limit`;
- file `.pdf` không có `pages`;
- ảnh (`png`, `jpg`, `jpeg`, `gif`, `webp`) trên 300 KB.

Khi bị chặn, agent thấy một dòng bắt đầu bằng `znf read-guard:` nêu kích cỡ và cách đọc lại: đọc theo lát với `offset` + `limit`, đọc PDF theo `pages`, hoặc giao một subagent tóm tắt và trả về vài dòng. Ảnh lớn nên để `znf:ui-verifier` xem và trả verdict.

Hook fail-open: file không tồn tại, payload lạ, hay lỗi bên trong đều cho qua. Nó chỉ chặn đúng ba trường hợp trên. Không có cờ bỏ qua trong prompt; nếu thật sự cần đọc nguyên file, bạn đọc thay bằng `! cat <file>` trong session, hoặc tắt hook trong `~/.claude/settings.json`. Số lần chặn được ghi vào meter của session dưới tên `Read:denied`, nhìn thấy trong `zenify cost`.

## Meter: hỏi một lần khi session nặng

Hook `PostToolUse` đo byte kết quả tool. Một kết quả trên 50 KB sinh một dòng nhắc chuyển output ra file. Khi tổng trong session vượt 2 MB, meter yêu cầu agent hoàn tất bước đang làm đến ranh giới file gần nhất (spec, plan, ledger) rồi hỏi bạn một câu: `/clear` và vào lại bằng đường dẫn file đó, hay tiếp tục. Câu này chỉ hỏi một lần cho mỗi session; sau đó meter im lặng.

## Statusline: ctx% đổi màu

`zenify observe statusline` in `ctx N%` theo `context_window.used_percentage` của Claude Code: thường dưới 50%, vàng từ 50%, đỏ từ 80%. Đỏ là lúc `/clear` ở ranh giới file gần nhất. Nếu bạn đã có statusline riêng, kit không ghi đè; dùng `--segment` để ghép hai segment của kit vào dòng của bạn.

## Quy tắc trả về của subagent

`znf:discipline` §10: mọi dispatch ghi rõ cap trả về, tối đa 40 dòng. Bảng, danh sách dài, log ghi ra file và trả đường dẫn. Đây là quy tắc bằng chữ, không có hook chặn; tin nhắn agent trả về là nguồn lớn thứ hai lấp context sau kết quả tool.

## Việc bạn tự làm

- Chạy `/context` một lần trên máy mình. MCP server nào bạn không dùng trong workspace này thì tắt; mỗi server chiếm vài nghìn token schema ở mọi turn.
- Auto-compact: kit không đặt ngưỡng. Trên model 1M, mặc định nổ ở khoảng 967k; `/autocompact 300k` hay thấp hơn là lựa chọn cá nhân, cân giữa số lần compact và kích cỡ context.
- `/clear` giữa hai việc không liên quan là miễn phí và hiệu quả nhất.
