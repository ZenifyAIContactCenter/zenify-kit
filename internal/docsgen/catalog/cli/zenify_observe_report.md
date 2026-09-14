---
summary: Tóm tắt số lần dispatch subagent và lượng tool output của từng phiên Claude Code.
---
## Khi nào dùng

Khi bạn muốn xem một phiên đã fan-out bao nhiêu subagent và tiêu tốn bao nhiêu tool output. Phiên hoạt động gần nhất hiện trước.

## Kết quả

Một bảng theo phiên, hoặc JSON khi thêm cờ. Dữ liệu đến từ hai hook `observe-count` và `observe-meter` mà `zenify up` đã cài.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--json` | In JSON thay cho bảng. |

## Ví dụ

```bash
zenify observe report
```
