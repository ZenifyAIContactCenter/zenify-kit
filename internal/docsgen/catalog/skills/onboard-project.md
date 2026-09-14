---
summary: Dựng hoặc vẽ lại bản đồ hệ thống của một project trước khi làm việc tính năng trên đó.
---
## Khi nào dùng

Khi bắt đầu làm việc trong một project chưa có CLAUDE.md, khi tham gia một codebase chưa quen, hoặc khi bắt đầu một project hoàn toàn mới. Phần hạ tầng chung của kit (worktree, hook, `/ship`, `/run`) đã sẵn sàng cho mọi project; skill này chỉ nối phần riêng của project đó — deploy branch, cách đọc DB, cách chạy app.

## Cách hoạt động

1. Skill kiểm tra thư mục hiện tại để chọn một trong hai chế độ: MAP nếu đã có code thật, BOOTSTRAP nếu thư mục còn trống hoặc chỉ có khung sườn.
2. Ở chế độ MAP, skill nhận diện stack, vẽ kiến trúc và các tài nguyên dùng chung giữa dịch vụ (DB, queue, kênh pub/sub, API), đánh giá lớp bảo vệ hiện có (test, CI, lint), rồi soạn cấu hình project — danh sách deploy branch, hook lint/typecheck — và viết CLAUDE.md. Mọi lệnh ghi vào CLAUDE.md phải được xác minh chạy được trước, không copy từ README hay repo khác.
3. Ở chế độ BOOTSTRAP, skill giúp chốt phạm vi nhỏ nhất hữu ích, đưa ra 2-3 lựa chọn stack/kiến trúc kèm đánh đổi để bạn chọn, dựng khung chạy được nhỏ nhất, rồi thiết lập lint, typecheck, một test mẫu và một CI gate cơ bản ngay từ ngày đầu.
4. CLAUDE.md luôn được đóng dấu ngày kèm commit đã xác minh, để một bản đồ đúng lúc viết nhưng đã lệch theo thời gian trở thành câu hỏi nhìn thấy được, không phải một lỗi âm thầm.

## Lưu ý

Năm điều sau luôn cần bạn xác nhận, skill không tự quyết: branch nào deploy, chuỗi kết nối DB chỉ đọc, sản phẩm public hay nội bộ, URL/đăng nhập để chạy app và có cần VPN không, và mọi quy ước riêng không thấy được từ code.
