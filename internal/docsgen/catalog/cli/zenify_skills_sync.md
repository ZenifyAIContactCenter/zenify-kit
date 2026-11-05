---
summary: Ghi bộ skill znf từ binary vào marketplace ~/.zenify/plugin-marketplace và gắn hook znf vào settings của workspace.
---
## Khi nào dùng

Sau mỗi lần `zenify update`, để skill trên máy khớp với binary. Cũng dùng khi một skill `znf:*` không xuất hiện trong Claude Code.

## Kết quả

Lệnh ghi cây plugin `znf` vào `~/.zenify/plugin-marketplace/plugins/znf` (hoặc `$ZENIFY_HOME/plugin-marketplace`) theo một manifest theo dõi từng file, rồi in số file đã ghi, giữ, không đổi và gỡ. File bạn đã sửa tay được giữ nguyên và không nhận bản vá tiếp theo. Khi chạy trong workspace, lệnh cập nhật plugin local nếu bản đã cài cũ hơn binary rồi gắn hook `znf` vào `<workspace>/.claude/settings.local.json`. Khi `~/.claude/settings.json` còn hook của kit (máy chưa chạy `zenify up --apply` bản này), lệnh không gắn hook vào workspace mà in `kit vẫn cài ở cấp user; chạy zenify up --apply để chuyển sang workspace`.

## Ví dụ

```bash
zenify skills sync
```

## Lưu ý

Muốn nhận lại bản chuẩn của một file đã sửa tay, xóa file đó rồi chạy lại. Cách kit phân lớp binary, skill và store: [Ba lớp của kit](/concepts/three-layers).
