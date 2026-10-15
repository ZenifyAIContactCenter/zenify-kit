---
summary: In header Authorization cho MCP http của Claude Code, dưới dạng một object JSON.
---
## Khi nào dùng

Bạn không cần gõ lệnh này. `zenify up` đăng ký hai MCP server `zenify-grafana` và `zenify-posthog` trong Claude Code với `headersHelper` là `zenify broker headers`, và Claude Code tự gọi lệnh mỗi lần kết nối. Claude Code bỏ helper sau 10 giây nên lệnh dùng cache token, và một giới hạn 8 giây bao cả `gh auth token` lẫn lời gọi HTTP.

## Kết quả

In đúng một object JSON `{"Authorization":"Bearer <JWT>"}` ra stdout và không in gì khác. Lỗi ra stderr với mã thoát khác 0. Manifest không khai báo `broker:` thì lệnh thoát mã 2. Chứa JWT, đừng dán kết quả vào chat hay log.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--manifest` | File manifest có `broker:` quyết định broker. Mặc định là manifest nhúng sẵn trong binary, không bao giờ là file trong thư mục hiện tại. |
| `--overlay` | File overlay cá nhân. Mặc định không có; overlay không đổi `broker:`. |

## Ví dụ

```bash
zenify broker headers
```
