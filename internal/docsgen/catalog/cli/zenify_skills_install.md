---
summary: Cài bộ coding skill khớp với repo hiện tại vào .claude/skills của repo.
---
## Khi nào dùng

Khi bạn vào một repo mà session chưa có skill về stack của repo đó, hoặc sau khi kit thêm skill mới cho stack này.

## Kết quả

Lệnh nhận diện repo qua dấu vết trên đĩa, chọn bộ skill tương ứng và ghi vào `.claude/skills` kèm manifest, rồi in số file ghi, giữ và không đổi. Repo không nằm trong bảng ánh xạ thì lệnh nói vậy và không ghi gì. Với một số stack, lệnh gợi ý thêm skill bên thứ ba dạng `npx skills add …` để bạn tự chạy và commit.

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
