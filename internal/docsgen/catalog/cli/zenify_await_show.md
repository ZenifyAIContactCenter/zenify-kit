---
summary: In lại bản ghi await theo id.
---
## Khi nào dùng

Khi cần xem một bản ghi await đã ghi, ví dụ worker host muốn đọc lại nội dung thẻ. Id là chuỗi `<kind>-<thời điểm UTC>-<8 ký tự hex>` mà `zenify await` in ra.

## Kết quả

In nguyên JSON của `$ZENIFY_HOME/await/<id>.json`. Id không tồn tại hoặc không phải tên file thuần thì lệnh thoát mã 2.

## Ví dụ

```bash
zenify await show spec-20261022T101500Z-1a2b3c4d
```
