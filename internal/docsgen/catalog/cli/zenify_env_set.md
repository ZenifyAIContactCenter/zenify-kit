---
summary: Nhập ẩn giá trị một key vào `.zenify/.env`, kèm dòng mô tả tuỳ chọn.
---
## Khi nào dùng

Thêm hoặc đổi một key mà không mở cả form. Cần terminal thật; stdin là pipe thì lệnh báo lỗi. Giá trị có ký tự xuống dòng bị từ chối.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--desc` | Ghi hoặc thay dòng mô tả phía trên key. |

## Ví dụ

```bash
zenify env set OPENROUTER_API_KEY --desc "key cá nhân"
```
