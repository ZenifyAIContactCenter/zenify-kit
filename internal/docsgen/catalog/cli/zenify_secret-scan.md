---
summary: Quét một cây thư mục để tìm secret bị lộ, dùng trong CI và kiểm tra tay trước khi push.
---
## Khi nào dùng

Trước khi push một branch có file cấu hình hoặc script mới, và trong CI của mọi repo public.

## Kết quả

Mỗi finding in ra một dòng gồm file, số dòng, tên rule và giá trị đã che. Lệnh không in secret nguyên văn. Có finding thì lệnh kết thúc lỗi để CI đỏ. Không truyền đường dẫn, lệnh quét thư mục hiện tại.

## Ví dụ

```bash
zenify secret-scan .
```

## Lưu ý

Một binary vừa build nằm trong cây thư mục có thể tạo finding giả. Xóa binary trước khi quét.
