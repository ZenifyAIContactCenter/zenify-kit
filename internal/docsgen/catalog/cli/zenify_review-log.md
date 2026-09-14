---
summary: Xem tổng hợp các lần review của `/znf:review` đã ghi lại trên máy bạn.
---
## Khi nào dùng

Khi bạn muốn biết review engine đã chạy bao nhiêu lần trong repo này, ở tier nào, và bao nhiêu finding được giữ lại hay bác bỏ.

## Kết quả

Bản tóm tắt gồm số lần review theo tier, số finding theo mức độ, tỷ lệ finding bị bác bỏ ở bước verify, số lần shippable và các nhóm finding gặp nhiều nhất. Dữ liệu nằm trong thư mục `.znf/review-log` của checkout chính, nên chạy từ worktree vẫn thấy cùng một log. Chưa có review nào thì lệnh in "no reviews logged yet".

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--json` | In toàn bộ record dạng JSON. |

## Ví dụ

```bash
zenify review-log
```
