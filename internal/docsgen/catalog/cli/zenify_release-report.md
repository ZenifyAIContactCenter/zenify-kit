---
summary: Sinh báo cáo rủi ro cho một release từ lịch sử git của mọi repo trong workspace.
---
## Khi nào dùng

Trước khi deploy một release, hoặc bất kỳ lúc nào bạn muốn xem release đang hình thành gồm những thay đổi gì.

## Kết quả

Lệnh chỉ đọc git và ghi một file markdown `R<N>.md` vào thư mục releases của knowledge store, rồi in đường dẫn file. Không truyền N thì lệnh lấy số release lớn nhất tìm thấy trong các repo. Không tìm được N thì lệnh báo và kết thúc bình thường.

Mỗi dòng trong báo cáo lấy metadata từ commit ghi chú của `zenify release-note`. Thay đổi có trong release nhưng chưa có trên `staging` được đánh dấu là regression.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--unreleased` | Ghi bản xem trước của release đang hình thành, từ release mới nhất tới `staging`, ra `unreleased.md`. |
| `--out-dir` | Thư mục ghi báo cáo. Mặc định thư mục releases của knowledge store. |
| `--workspace` | Thư mục workspace. Mặc định thư mục hiện tại. |
| `--no-fetch` | Không fetch, dùng ref local. |
| `--verbose` | Hiện cả commit chore và chi tiết từng commit. |

## Ví dụ

```bash
zenify release-report 42
zenify release-report --unreleased
```
