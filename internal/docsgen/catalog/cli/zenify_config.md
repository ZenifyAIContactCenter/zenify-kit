---
summary: Kéo cấu hình chung của team từ knowledge store vào workspace theo một chiều.
---
## Khi nào dùng

Sau khi team đổi CLAUDE.md gốc, system map hoặc bộ rule chung. `zenify up` đã gọi lệnh này, bạn chỉ chạy tay khi muốn cập nhật giữa hai lần onboard.

## Kết quả

Nguồn là thư mục `.config/` trong knowledge store, với danh sách file phân phối ghi ở `distribution.txt`. Mặc định lệnh in từng file kèm trạng thái, đích và diff, rồi tóm tắt số file đổi, giữ nguyên và bỏ. Với `--apply` lệnh ghi. Không thấy manifest, lệnh in ghi chú và thoát bình thường.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--apply` | Ghi file. Không có cờ này lệnh chỉ in diff. |
| `--config-dir` | Thư mục nguồn thay cho `.config/` trong store. |
| `--workspace` | Thư mục workspace đích. |

## Ví dụ

```bash
zenify config
zenify config --apply
```

## Lưu ý

Chiều đi luôn từ store vào workspace. Sửa tại workspace sẽ bị ghi đè ở lần apply sau, nên đổi cấu hình chung ở store. Xem [Knowledge store](/concepts/knowledge-store).
