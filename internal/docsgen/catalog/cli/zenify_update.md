---
summary: Nâng cấp binary ZenifyKit lên bản mới nhất bằng đúng kênh đã cài, hoặc chỉ kiểm tra có bản mới.
---
## Khi nào dùng

Khi đầu session in dòng `zenify: vX is available`. Hoặc khi tài liệu nhắc một lệnh mà binary của bạn báo không biết.

## Kết quả

Lệnh nhận ra cách bạn đã cài qua vị trí binary rồi gọi đúng lệnh nâng cấp: Homebrew, Scoop, hoặc chạy lại script cài đặt. Không nhận ra được, lệnh in cả ba cách để bạn tự chọn và không làm gì.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--check` | Chỉ so phiên bản đang chạy với bản mới nhất và in kết quả, không nâng cấp. |

## Ví dụ

```bash
zenify update --check
zenify update
```

## Lưu ý

Đặt `ZENIFY_NO_UPDATE_CHECK=1` để tắt dòng nhắc đầu session. Chi tiết theo từng kênh cài: [Nâng cấp](/getting-started/upgrade).
