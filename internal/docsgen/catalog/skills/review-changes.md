---
summary: Review đối kháng nhiều khía cạnh cho diff lớn, xác minh chéo từng phát hiện nghiêm trọng.
---
## Khi nào dùng

Đây là lớp review sâu nhất bên trong `/znf:review`, tự động chạy khi diff đủ lớn hoặc đủ nhạy cảm. Bạn hiếm khi cần gọi trực tiếp — dùng cho một diff lớn hơn khoảng 200 dòng, hoặc trước khi merge một nhánh chạm tới hợp đồng dùng chung, khi bạn muốn một góc nhìn độc lập, tốn nhiều token hơn cách review thông thường.

## Cách hoạt động

1. Skill lấy diff cần review, cùng một chút bối cảnh về ý định của thay đổi nếu có.
2. Skill dàn trải review qua năm khía cạnh — lỗi, bảo mật, hiệu năng, hợp đồng, kiểu dữ liệu — rồi xác minh đối kháng riêng các phát hiện mức nghiêm trọng cao nhất bằng nhiều người kiểm độc lập; chỉ phát hiện được đa số xác nhận mới giữ lại.
3. Phát hiện mức trung bình được trả về dạng tư vấn, không qua vòng xác minh đối kháng; phát hiện mức thấp trả về riêng, không chặn gì.
4. Mọi phát hiện Critical và High đã xác nhận cần được sửa trước khi ship.

## Lưu ý

Chi phí mỗi lượt chạy đáng kể vì nhiều reviewer và nhiều vòng xác minh cùng làm việc trên diff. Với diff nhỏ, một reviewer độc lập đơn lẻ thường đủ và rẻ hơn nhiều.
