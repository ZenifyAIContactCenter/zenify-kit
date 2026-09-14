---
summary: Ghi bộ skill znf từ binary vào ~/.claude/skills/znf và gắn hook znf vào settings.
---
## Khi nào dùng

Sau mỗi lần `zenify update`, để skill trên máy khớp với binary. Cũng dùng khi một skill `znf:*` không xuất hiện trong Claude Code.

## Kết quả

Lệnh ghi cây `znf/` theo một manifest theo dõi từng file, rồi in số file đã ghi, giữ, không đổi và gỡ. File bạn đã sửa tay được giữ nguyên và không nhận bản vá tiếp theo. Sau đó lệnh gắn hook `znf` vào `~/.claude/settings.json`.

## Ví dụ

```bash
zenify skills sync
```

## Lưu ý

Muốn nhận lại bản chuẩn của một file đã sửa tay, xóa file đó rồi chạy lại. Cách kit phân lớp binary, skill và store: [Ba lớp của kit](/concepts/three-layers).
