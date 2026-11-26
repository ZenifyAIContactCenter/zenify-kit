---
summary: Xem hoặc đặt domain và số env tối đa của devbox k3s cục bộ.
---
## Khi nào dùng

Khi domain mặc định `zenify.test` không hợp (ví dụ hai devbox cùng mở ra một LAN), hoặc muốn giới hạn số env trên máy.

## Kết quả

Không có cờ đặt giá trị, lệnh in `domain`, `maxEnvs`, `chartRef` hiện tại; `domain` và `chartRef` để trống nghĩa là `zenify infra up` theo platform đang chạy. Có cờ, lệnh kiểm rồi ghi `~/.zenify/infra/config.json` (quyền 0600). Domain phải là nhãn DNS chữ thường và kết thúc bằng `.test`; `--max-envs` phải từ 1 trở lên. `maxEnvs` nhỏ hơn 1 trong file config cũng bị báo lỗi kèm đường dẫn file, ở mọi lệnh `zenify infra`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--local` | Bắt buộc: nhắm devbox k3s cục bộ. |
| `--domain` | Domain của các env, ví dụ `quyentm.test`. |
| `--max-envs` | Số env tối đa trên devbox, tối thiểu 1. |

## Ví dụ

```bash
zenify infra config --local
zenify infra config --local --domain quyentm.test --max-envs 2
```
