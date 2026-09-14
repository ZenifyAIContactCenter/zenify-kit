---
summary: Đọc/ghi MongoDB an toàn trong một codebase đa tenant không có schema cứng.
---
## Khi nào dùng

Khi đọc hoặc ghi document MongoDB trong một codebase đa tenant, không có schema bắt buộc. Cần cài đặt trước bằng lệnh cài skill coding.

## Cách hoạt động

1. Skill nhắc MongoDB không có row-level security: mọi truy vấn và mọi stage aggregation chạm một collection dùng chung phải tự mang bộ lọc tenant trong code ứng dụng, vì thiếu bộ lọc đó vẫn trả kết quả trông có vẻ đúng trên dữ liệu dev một tenant, rồi rò rỉ dữ liệu tenant khác trên production.
2. Skill nhắc một schema khai `strict: false` vẫn ghi im lặng mọi field không khai báo, và truy cập qua driver thô bỏ qua hoàn toàn lớp schema — nên "schema không khai field đó" không nói lên điều gì về dữ liệu thật.
3. Skill nhắc chạy `distinct()` trên một field trước khi viết nhánh rẽ theo giá trị của nó, vì dữ liệu cũ hoặc do dịch vụ khác ghi có thể mang giá trị mà nhánh mới chưa xử lý.
4. Skill nhắc luôn liệt kê tên collection thật từ DB trước khi dùng, vì tên hiển thị trong code và tài liệu thường là tên model chứ không phải tên collection thật, và MongoDB tạo collection mới im lặng ngay lần ghi đầu nếu tên gõ sai.

## Lưu ý

Trước khi đổi hình dạng một collection dùng chung giữa nhiều dịch vụ, chạy `/znf:gate`. Trước khi khẳng định đã xong, xác minh bằng đường code thật qua `/znf:ship`.
