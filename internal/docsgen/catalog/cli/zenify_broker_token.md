---
summary: In JWT của credential broker ra stdout, dùng cache và tự xin lại khi còn dưới 60 giây.
---
## Khi nào dùng

Khi cần kiểm tra bạn đã đăng nhập được vào broker chưa, hoặc cần token cho một công cụ khác. Lệnh đổi GitHub token của bạn (lấy từ `gh auth token`) lấy JWT sống 15 phút, rồi lưu ở `~/.zenify/broker/token.json` với quyền 0600.

## Kết quả

In đúng một dòng là JWT. Đây là lệnh duy nhất cố ý in token; đừng dán kết quả vào chat hay log. Manifest không khai báo `broker:` thì lệnh thoát mã 2 với thông báo "manifest không khai báo broker".

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--manifest` | File manifest repos thay cho `manifest/repos.yaml` hoặc bản nhúng sẵn. |
| `--overlay` | File overlay cá nhân. Mặc định `<workspace>/.zenify-overlay.yaml`. |

## Ví dụ

```bash
zenify broker token
```
