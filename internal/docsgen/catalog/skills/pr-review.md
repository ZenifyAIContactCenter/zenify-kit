---
summary: Vòng xử lý review của bot sau khi PR đã mở: chờ đúng check review mà project khai báo trên commit cuối, sửa hoặc decline những gì check đó chặn, push rồi chờ lại, tối đa 3 vòng; không bao giờ merge.
---
## Khi nào dùng

`/znf:ship` tự gọi skill này ngay sau `gh pr create`. Bạn cũng gọi tay được trên một PR có sẵn: `/znf:pr-review <số PR hoặc link>` khi muốn Claude xử lý review của bot hoặc chờ kết luận trên commit cuối. Khi gọi tay, bạn phải đang đứng trên đúng branch của PR, ở đúng commit cuối của nó.

## Cách hoạt động

1. Skill đi ngược từ worktree lên, đọc từng `CLAUDE.md`; dòng đầu tiên bắt đầu bằng `PR review check:` cho biết tên check, nơi bot ghi finding, cú pháp decline, lệnh chấm lại, và (nếu có) tên tool để chờ, để decline và để chấm lại. Không có dòng đó thì skill bỏ qua vòng lặp, báo URL PR (hoặc chạy `/znf:ci-watch` khi có `--watch`).
2. Skill chọn đường trao đổi với bot:
   - **Qua tool**: dòng khai báo có đủ ba tool và phiên có các tool đó. Chờ kết quả, decline và yêu cầu chấm lại đều đi qua tool; skill không đăng gì lên PR (không dòng decline, không lệnh chấm lại, không trả lời thread). Bot tự đăng báo cáo của nó.
   - **Qua comment**: thiếu tool hoặc gọi lỗi. Skill dùng cú pháp comment trong dòng khai báo; sau mỗi lần push, nó trả lời từng review thread mà commit đó đã xử lý, và resolve thread do bot mở. Thread do người mở thì chỉ trả lời, không resolve.
3. Skill chờ check đó trên đúng commit vừa push, tối đa 30 phút, không chờ các check khác của PR. Xanh hay đỏ luôn là `conclusion` của check trên GitHub, không suy từ danh sách finding hay kết quả tool.
4. Check đỏ: skill đọc đoạn code mà từng finding chỉ tới trước khi sửa. Finding CRITICAL/HIGH đúng thì sửa, gộp cả CI đang đỏ, trong MỘT commit; finding sai thì decline kèm bằng chứng, không đổi code; MEDIUM thì sửa chung commit hoặc decline kèm lý do, không push một vòng chỉ để sửa MEDIUM.
5. Trước mỗi lần push: `git fetch` (branch remote đã đổi thì rebase, không force-push), chạy lại lint, build/typecheck và test liên quan theo bước 2 của `/znf:ship`, rồi `/znf:review` trên phần vừa sửa.
6. Dừng khi check xanh (báo các finding không chặn còn mở, không push thêm vì chúng), hoặc sau 3 vòng vẫn đỏ (báo từng finding còn mở và những gì đã thử).

## Lưu ý

- Skill không nhắc tên bot hay tên tool nào: tất cả lấy từ dòng `PR review check:` của project.
- Một vòng là một lần push, hoặc một lần decline hay yêu cầu chấm lại mà không push gì. Decline gửi kèm một lần push không tính thêm vòng.
- Người khác push vào branch trong lúc chờ thì skill dừng và báo.
- Skill không merge. Lệnh merge chỉ được in ra khi check review đã xanh trên commit cuối hiện tại của PR.
- Có `--watch` (hoặc feature `track` đang bật) thì ở mọi điểm dừng skill vẫn chạy `/znf:ci-watch` trước khi báo cáo.
