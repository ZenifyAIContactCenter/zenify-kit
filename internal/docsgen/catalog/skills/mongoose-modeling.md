---
summary: Thiết kế schema Mongoose an toàn — index, lean/populate, discriminator, thứ tự thêm field mới.
---
## Khi nào dùng

Khi thiết kế hoặc sửa một schema Mongoose. Cần cài đặt trước bằng lệnh cài skill coding.

## Cách hoạt động

1. Skill hướng dẫn đặt index cho mọi field một truy vấn lọc hoặc sắp xếp theo, và cách sắp thứ tự field trong index gộp sao cho khớp thứ tự truy vấn thật sự lọc — thiếu index không lộ ra trên DB dev nhỏ nhưng chậm ngay khi dữ liệu lớn lên.
2. Skill phân biệt khi nào dùng `.lean()` cho đường chỉ đọc để trả response nhanh hơn, và khi nào cần giữ document đầy đủ vì còn gọi phương thức hoặc lưu lại; và khi nào `.populate()` một field chỉ để đọc một thuộc tính nên thay bằng một stage `$lookup` hoặc chỉ định rõ field cần lấy.
3. Skill mô tả khi nào dùng discriminator để nhiều dạng document chia sẻ một collection nhưng vẫn giữ field riêng cho từng dạng, và khi nào một field đóng vai trò phân loại mềm là đủ, không cần discriminator đầy đủ.
4. Skill nêu thứ tự an toàn khi thêm một field bắt buộc vào schema đã có dữ liệu: thêm tuỳ chọn trước, chuyển các nơi ghi sang điền field đó, backfill dữ liệu cũ theo lô, rồi mới đánh dấu bắt buộc — bỏ qua thứ tự này phá vỡ mọi document cũ ngay khi nó được đọc rồi lưu lại.

## Lưu ý

Đổi hình dạng một collection dùng chung giữa nhiều dịch vụ cần chạy `/znf:gate`. Các bẫy khi đọc/ghi runtime trên một schema đã thiết kế xong (bộ lọc tenant, `strict: false`, truy cập qua driver thô) thuộc về skill `mongo-data-safety`.
