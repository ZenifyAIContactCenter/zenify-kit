---
summary: Xem hoặc bật/tắt các feature của kit (ví dụ track) ngay trong Claude Code, qua lệnh `zenify feature`.
---
## Khi nào dùng

Khi bạn muốn bật hoặc tắt một feature của kit (ví dụ `track`) hoặc xem feature nào đang bật mà không phải mở terminal.

## Cách hoạt động

1. Không đối số (`/znf:features`): skill chạy `zenify feature` và in danh sách `key on|off`.
2. Có đối số (`/znf:features track on`): skill chạy `zenify feature <key> on|off`, rồi chạy `zenify feature` để in trạng thái mới.
3. Skill in output nguyên văn. Key hoặc giá trị sai thì in lỗi nguyên văn và dừng.
4. Sau khi bật `track`, status line tự cập nhật trong vài giây. Workspace chưa onboard thì digest của session sau sẽ nhắc.

## Lưu ý

Skill chỉ gọi `zenify feature`: không sửa file overlay bằng tay, không chạy lệnh khác, không dispatch agent. Lệnh ghi vào overlay của workspace nên áp dụng cho mọi session trong workspace đó.
