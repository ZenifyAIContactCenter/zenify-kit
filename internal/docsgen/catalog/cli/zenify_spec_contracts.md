---
summary: Liệt kê repo và collection mà từng spec khai qua hai tag `_Blast-radius:` và `_DB:`.
---
## Khi nào dùng

Trước khi sửa một collection chung, để xem spec nào đã khai chạm vào nó.

## Kết quả

Một dòng mỗi spec gồm repo, blast radius, DB và đường dẫn spec. Spec thiếu hai tag được đếm và bỏ qua.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--repo` | Chỉ hiện spec khai repo này trong `_Blast-radius:`. |
| `--collection` | Chỉ hiện spec khai collection này trong `_DB:`. |
| `--json` | In JSON thay cho bảng. |
| `--workspace` | Thư mục workspace. Mặc định thư mục hiện tại. |

## Ví dụ

```bash
zenify spec contracts --collection chat_rooms
```
