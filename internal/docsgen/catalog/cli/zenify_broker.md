---
summary: Nhóm lệnh làm việc với credential broker của zenify.
---
## Khi nào dùng

Khi manifest của team khai báo `broker:`. Hiện có một lệnh con là `zenify broker token`; các lệnh khác của kit dùng broker qua cùng cache token này.

## Kết quả

Manifest không khai báo `broker:` thì mọi lệnh kit giữ nguyên hành vi trước đây.
