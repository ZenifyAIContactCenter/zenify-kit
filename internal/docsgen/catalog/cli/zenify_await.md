---
summary: Dừng một lượt chạy không người ở cổng cần người duyệt, ghi bản ghi await rồi thoát mã 8.
---
## Khi nào dùng

Chỉ trong chế độ không người (`ZNF_AUTONOMOUS=1`). Khi worker gặp cổng cần người, như duyệt spec hoặc plan trong `/cook`, hay một lệnh ghi dữ liệu prd mà chỉ lead được chạy, nó gọi `zenify await <kind>` thay vì tự quyết. Ngoài chế độ không người, lệnh từ chối với mã 2.

Năm `kind` hợp lệ: `spec`, `plan`, `data-write`, `irreversible`, `needs-human`. Kind khác bị từ chối với mã 2; `--summary` trống, `--file` trỏ file không tồn tại, hay `data-write` mà `--command` không chứa `zenify data run --prd --write` cũng vậy.

## Kết quả

Lệnh ghi `$ZENIFY_HOME/await/<id>.json` (thư mục mode 0700, file mode 0600) qua file tạm `.<id>.json.tmp` rồi đổi tên, nên host không bao giờ đọc phải bản ghi dở, in `<id>` ra stdout rồi thoát mã 8. Mã 8 là điểm dừng có kế hoạch, không phải lỗi, nên thông điệp trên stderr không có tiền tố `error:`. Worker host đọc bản ghi và hiện thẻ cho người duyệt.

Bản ghi gồm `id`, `kind`, `summary`, `files` (đường dẫn tuyệt đối), `command`, `task` (lấy từ `ZNF_TASK_ID` khi khớp `^[A-Za-z0-9._-]{1,64}$`, không thì để rỗng), `repo`, `branch`, `cwd`, `created_at`. Ngoài git repo thì `repo` và `branch` để rỗng.

## Ví dụ

```bash
zenify await spec --summary "Spec cho tính năng X" --file docs/specs/zenify-kit/x-design.md
zenify await data-write --summary "Backfill field y" --command 'cd /abs/repo && zenify data run --prd -- node y.js --dry-run && zenify data run --prd --write -- node y.js'
```
