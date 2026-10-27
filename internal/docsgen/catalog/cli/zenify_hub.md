---
summary: Mở phiên tmux có sidebar theo dõi task, agent và PR của workspace.
---
## Khi nào dùng

Khi chạy nhiều agent Claude cùng lúc và muốn một màn hình xem task nào đang chạy, agent nào đang chờ bạn, PR nào đã sẵn sàng.

## Kết quả

Lệnh tạo (hoặc nối lại) phiên tmux `zenify`: cửa sổ `agent` chạy `claude` ở bên phải, sidebar `zenify hub sidebar` chiếm 30% bên trái. Đang ở trong tmux thì lệnh chỉ nhắc chạy `zenify hub sidebar` trong một pane. Cần tmux 3.1 trở lên; thiếu thì lệnh báo cách cài.

## Ví dụ

```bash
zenify hub
```

## Lưu ý

Phải chạy từ trong workspace zenify. Dùng `zenify doctor` để kiểm tra tmux.
