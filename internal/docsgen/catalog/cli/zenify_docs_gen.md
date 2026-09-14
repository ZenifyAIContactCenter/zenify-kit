---
summary: Sinh các trang tham chiếu của website này từ binary, hoặc kiểm tra trang đã sinh còn khớp.
---
## Khi nào dùng

Khi bạn sửa một lệnh, một skill hoặc một hook trong kit và cần trang tham chiếu đi theo. CI chạy bản `--check` trên mỗi PR.

## Kết quả

Lệnh ghi markdown vào thư mục đích, mặc định `website/reference`. Với `--check` lệnh chỉ so sánh: khớp thì im lặng, lệch thì liệt kê file và trả mã thoát khác 0.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--check` | So nội dung sẽ sinh với file trên đĩa, không ghi. |
| `--out` | Thư mục đích, mặc định `website/reference`. |

## Ví dụ

```bash
zenify docs gen
zenify docs gen --check
```
