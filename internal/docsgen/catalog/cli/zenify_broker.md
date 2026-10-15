---
summary: Nhóm lệnh làm việc với credential broker của zenify.
---
## Khi nào dùng

Khi manifest của team khai báo `broker:`. Hiện có hai lệnh con là `zenify broker token` và `zenify broker headers`; các lệnh khác của kit dùng broker qua cùng cache token này.

## Kết quả

Manifest không khai báo `broker:` thì mọi lệnh kit giữ nguyên hành vi trước đây.
