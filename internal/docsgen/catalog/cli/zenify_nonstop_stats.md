---
summary: Tính thang size S/M/L đã học và thống kê hôm nay, 7 ngày, luật calibration từ estimates.md của nonstop.
---
## Khi nào dùng

Khi lập kế hoạch buổi sáng và khi đóng ngày trong `znf:nonstop`: dùng `scale` để định size task, dùng `today`, `week`, `rules` cho phần thống kê và dòng Calibration thay vì tự tính tay. Pane cũng gọi lệnh này với `--json`.

## Kết quả

Đọc `<dir>/estimates.md`, mặc định `<workspace>/.zenify/nonstop`. Thang thời gian học từ tối đa 60 task gần nhất có số phút, lấy phân vị 25/50/75 (nearest-rank); dưới 20 task thì dùng mặc định 5/20/60 phút. Size thực tế của một dòng là size lớn hơn giữa thời gian theo thang và số file (≤3 S, ≤10 M, ≤30 L, còn lại XL). Dòng sai format được đếm vào `malformed`.

`--json` in một object gồm `missing`, `scale`, `today`, `week`, `rules`, `malformed`. Thiếu `estimates.md` không phải lỗi: `missing` là `true` và lệnh thoát mã 0. Lỗi đọc file khác thì thoát mã khác 0. Không có `--json` thì in bảng chữ ngắn.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--dir` | Thư mục chứa `estimates.md`. Mặc định `<workspace>/.zenify/nonstop`. |
| `--json` | In JSON thay vì bảng chữ. |
| `--today` | Ngày báo cáo, `YYYY-MM-DD`. Mặc định là ngày local. |

## Ví dụ

```bash
zenify nonstop stats --json --dir .zenify/nonstop
```
