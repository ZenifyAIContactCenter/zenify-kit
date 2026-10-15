---
summary: Chạy một lệnh Redis chỉ đọc qua credential broker.
---
## Khi nào dùng

Khi cần xem giá trị thật trong Redis (key, hash, list, stream) của staging hoặc production mà không giữ mật khẩu Redis trên máy. Lệnh chỉ chạy được khi manifest khai báo `broker:`; không có thì lệnh thoát mã 2 với thông báo "redis-read cần broker".

## Kết quả

In kết quả của lệnh Redis dưới dạng JSON có thụt dòng.

Broker giới hạn mỗi lần đọc: `LRANGE`, `ZRANGE`, `XRANGE` trả tối đa 200 phần tử; kết quả tối đa 1 MiB, quá thì bị cắt và lệnh ghi thông báo ra stderr; `MGET` tối đa 100 key; `HGETALL`, `HKEYS`, `SMEMBERS` từ chối key có hơn 10000 phần tử.

Chỉ các lệnh sau được phép: `GET MGET HGET HGETALL HKEYS LRANGE LLEN SMEMBERS SCARD ZRANGE ZCARD XRANGE TTL TYPE EXISTS SCAN`. Lệnh khác bị từ chối ngay ở máy bạn (mã 2), không gửi lên broker. `SCAN` tự thêm `COUNT 100` nếu thiếu, và `COUNT` phải trong khoảng 1..1000.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--prd` | Đọc Redis production thay cho staging. |
| `--manifest` | File manifest có `broker:` quyết định broker. Mặc định là manifest nhúng sẵn trong binary, không bao giờ là file trong thư mục hiện tại. |
| `--overlay` | File overlay cá nhân. Mặc định không có; overlay không đổi `broker:`. |

Cờ phải đứng trước lệnh Redis, để tham số như `-1` không bị đọc thành cờ.

## Ví dụ

```bash
zenify redis-read GET some:key
zenify redis-read --prd LRANGE some:list 0 49
```
