---
summary: Quản lý biến môi trường cá nhân cho tool dev và agent trong `<workspace>/.zenify/.env`, không phải env chạy app.
---
## Khi nào dùng

Khi `zenify db-read`, `zenify e2e`, `zenify visual` hoặc một script tay cần credential. File nằm ở `<workspace>/.zenify/.env` với quyền 0600; env hiện có của tiến trình luôn thắng giá trị trong file, trừ khi giá trị đó rỗng.

## Kết quả

Không đối số, lệnh mở form nhập ẩn: hỏi trước các key toàn cục còn trống (trừ nhóm production), rồi cho chọn bất kỳ key nào để điền hoặc sửa. Form cần terminal thật.

## Ví dụ

```bash
zenify env
zenify env list
```
