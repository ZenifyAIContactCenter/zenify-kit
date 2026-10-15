---
summary: In JWT của credential broker ra stdout, dùng cache và tự xin lại khi còn dưới 60 giây.
---
## Khi nào dùng

Khi cần kiểm tra bạn đã đăng nhập được vào broker chưa, hoặc cần token cho một công cụ khác. Lệnh đổi GitHub token của bạn (lấy từ `gh auth token`) lấy JWT sống 15 phút, rồi lưu ở `~/.zenify/broker/token.json` với quyền 0600, kèm URL broker đã cấp; cache của broker khác không được dùng lại. GitHub token chỉ gửi tới host broker của manifest nhúng sẵn, hoặc của file bạn chỉ bằng `--manifest`. Ghi cache bị chặn (ví dụ sandbox Bash của Claude Code chặn `~/.zenify`) thì lệnh chỉ cảnh báo ra stderr và vẫn in token.

## Kết quả

In đúng một dòng là JWT. Lệnh này và `zenify broker headers` đều in một JWT còn hiệu lực; đừng dán kết quả vào chat hay log. Manifest không khai báo `broker:` thì lệnh thoát mã 2 với thông báo "manifest không khai báo broker".

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--manifest` | File manifest có `broker:` quyết định broker. Mặc định là manifest nhúng sẵn trong binary, không bao giờ là file trong thư mục hiện tại. |
| `--overlay` | File overlay cá nhân. Mặc định không có; overlay không đổi `broker:`. |

## Ví dụ

```bash
zenify broker token
```
