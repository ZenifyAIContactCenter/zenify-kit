---
summary: Bộ quy tắc làm việc thường trực của kit: định tuyến theo blast radius, không bịa, verify trước khi báo xong, worktree cho mọi thay đổi, git safety.
---
## Khi nào dùng

Khi bạn cần toàn văn và lý do của các quy tắc mà digest đầu phiên nhắc tới. Khi agent phân vân về sàn kỷ luật: worktree trước khi sửa, verify trước khi báo xong, không bịa tên.

## Cách hoạt động

Skill không chạy lệnh. Skill trình bày mười nhóm quy tắc mà agent phải theo:

1. Định tuyến trước khi sửa file bằng hai tiêu chí độc lập: blast radius quyết định fetch base, worktree, contract gate và ship; điều chưa biết quyết định skill, theo bảng ở [Chọn workflow](/workflows/). Bỏ skill chỉ khi việc nằm trong một repo, không chạm tài nguyên chung, bạn đã nêu cả yêu cầu lẫn lý do, và không phải lỗi production.
2. Không bịa: không nhắc file, hàm, field, endpoint chưa đọc trong phiên; khẳng định không hiển nhiên phải kèm `file:line`.
3. Diff nhỏ nhất thỏa yêu cầu, không thêm abstraction hay xử lý lỗi ngoài yêu cầu.
4. Verify trước khi nói xong, bằng output thật; thay đổi giao diện phải chụp màn hình và đo phần tử so với container; agent được dispatch mà im lặng không tính là kết quả sạch.
5. Ghi memory theo quy tắc, không theo sự việc; giữ danh sách việc cho công việc trên ba bước; commit và push tự do trên feature branch, không bao giờ commit, push hay merge vào deploy branch; mở PR nhưng không merge; mọi thay đổi code trong worktree, một worktree mỗi slug, base đọc từ config của repo; fact ngoài repo phải tìm trước và ghi nguồn; nghiên cứu tách được thì chia agent song song có giới hạn.

## Lưu ý

Xem thêm [Mỗi slug một worktree](/concepts/worktree-per-slug). `/znf:cook` chỉ được đề xuất, không tự chạy; `/znf:fix` và sàn kỷ luật thì agent bắt đầu ngay.
