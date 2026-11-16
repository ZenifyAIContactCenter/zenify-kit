---
summary: Ghi `node_modules` của một thư mục vào cache deps, chỉ khi cây khớp `package-lock.json`.
---
## Khi nào dùng

Ngay sau khi `npm ci` cài xong trong `<dir>` và bạn muốn lần sau (hoặc máy khác dùng chung cache) lấy lại được mà không cài lại.

## Kết quả

Lệnh chép `<dir>/node_modules` và `<dir>/package-lock.json` vào slot `<cache>/<tên repo>/<hash>/`, rồi giữ tối đa 5 slot mỗi repo (xoá slot ít dùng gần đây nhất trước; dọn slot lỗi chỉ in cảnh báo ra stderr, lệnh vẫn thoát 0). Slot đã có thì lệnh thoát 0 và không ghi lại. Tên repo lấy từ `--name`, không phải tên thư mục `<dir>`, nên checkout đặt tên khác vẫn dùng chung slot.

Lệnh thoát 0 khi ghi xong hoặc slot đã có, khác 0 khi lỗi. Lệnh từ chối ghi khi: `--name` sai dạng, `<dir>/node_modules` là symlink, `node_modules` không khớp `package-lock.json` (cùng phép kiểm mà `wt new` dùng trước khi ghi cache), npm đặt `ignore-scripts=true` hoặc không đọc được giá trị này, không dò được môi trường Node, hoặc cache đang tắt (`ZENIFY_DEPS_CACHE=off`).

`put` tin nội dung cây: nó chỉ kiểm cây khớp lock, không kiểm từng file. Chỉ chạy `put` trên cây chính bạn cài (sandbox worker chạy nó trong container riêng, không mạng, không token).

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--name` | Tên repo trong cache, dạng `[A-Za-z0-9][A-Za-z0-9._-]*` (một đoạn đường dẫn, không `..`). Mặc định là tên thư mục của `<dir>`. |

## Ví dụ

```bash
cd repo && npm ci
zenify wt cache put . --name contact-center-hub
```
