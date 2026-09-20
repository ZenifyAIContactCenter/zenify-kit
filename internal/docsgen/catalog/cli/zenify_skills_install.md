---
summary: Gỡ bản coding skill cũ khỏi .claude/skills của repo — chúng đã nằm trong plugin znf.
---
## Khi nào dùng

Một lần cho mỗi repo sau khi nâng kit lên bản có coding skill trong plugin `znf`: repo còn giữ bản cũ dưới `.claude/skills` (kèm `.manifest.json`) sẽ có hai skill cùng nội dung trong registry.

## Kết quả

Lệnh đọc `.claude/skills/.manifest.json`, xoá những file kit từng ghi mà bạn chưa sửa, giữ file bạn đã sửa và báo lại, không đụng file không có trong manifest; manifest rỗng thì xoá luôn. Repo không có manifest thì lệnh nói vậy và thoát 0. Với một số stack, lệnh vẫn gợi ý skill bên thứ ba dạng `npx skills add …` để bạn tự chạy và commit. Bản dùng chung lấy qua `zenify skills sync`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--dest` | Thư mục đích thay cho `.claude/skills` của repo. |
| `--repo` | Đường dẫn repo, khi không đứng trong repo đó. |

## Ví dụ

```bash
cd repos/contact-center-web
zenify skills install
```
