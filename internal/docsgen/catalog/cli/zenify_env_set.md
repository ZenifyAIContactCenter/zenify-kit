---
summary: Nhập ẩn giá trị một key vào `.zenify/.env`, kèm dòng mô tả tuỳ chọn.
---
## Khi nào dùng

Thêm hoặc đổi một key mà không mở cả form. Cần terminal thật; stdin là pipe thì lệnh báo lỗi. Giá trị có ký tự xuống dòng bị từ chối, và giá trị trống bị từ chối khi giá trị hiện tại cũng trống.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--desc` | Ghi hoặc thay dòng mô tả phía trên key. Bắt buộc với key ngoài manifest, trừ khi file đã có mô tả của key đó. |

## Ví dụ

```bash
zenify env set OPENROUTER_API_KEY --desc "key cá nhân"
```
