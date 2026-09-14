---
summary: Đồng bộ knowledge store với remote qua git và dựng lại view docs/ trong workspace.
---
## Khi nào dùng

Hook `SessionStart` và `Stop` gọi lệnh này thay bạn. Chạy tay khi bạn vừa tắt hook, hoặc muốn đẩy spec mới lên ngay.

## Kết quả

Lệnh kiểm tra trạng thái store trước: sạch thì không chạm mạng. Có thay đổi thì commit, `pull --rebase`, rồi push. Gặp conflict lệnh hủy rebase và giữ commit cục bộ để lượt sau thử lại. Mọi lỗi mạng đều fail-open: lệnh in cảnh báo và thoát bình thường. Sau đó lệnh dựng lại các symlink trong `docs/` của workspace trỏ vào store.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--dir` | Đường dẫn store, khi không dùng `~/.zenify/knowledge`. |
| `--workspace` | Thư mục workspace để dựng view `docs/`. |

## Ví dụ

```bash
zenify docs sync
```

## Lưu ý

Không chạy git tay trong store. Xem [Knowledge store](/concepts/knowledge-store) và [Gate fail-open](/concepts/gate-fail-open).
