---
summary: Dừng một lượt chạy không người ở cổng cần người duyệt, ghi bản ghi await rồi thoát mã 8.
---
## Khi nào dùng

Chỉ trong chế độ không người (`ZNF_AUTONOMOUS=1`). Khi worker gặp cổng cần người, như duyệt spec hoặc plan trong `/cook`, hay một lệnh ghi dữ liệu prd mà chỉ lead được chạy, nó gọi `zenify await <kind>` thay vì tự quyết. Ngoài chế độ không người, lệnh từ chối với mã 2.

Sáu `kind` hợp lệ: `spec`, `plan`, `data-write`, `irreversible`, `needs-human`, `question`. Kind khác bị từ chối với mã 2; `--summary` trống, `--file` trỏ file không tồn tại, hay `data-write` mà `--command` không chứa `zenify data run --prd --write` cũng vậy.

## Kết quả

Lệnh ghi `$ZENIFY_HOME/await/<id>.json` (thư mục mode 0700, file mode 0600) qua file tạm `.<id>.json.tmp` rồi đổi tên, nên host không bao giờ đọc phải bản ghi dở, in `<id>` ra stdout rồi thoát mã 8. Mã 8 là điểm dừng có kế hoạch, không phải lỗi, nên thông điệp trên stderr không có tiền tố `error:`. Worker host đọc bản ghi và hiện thẻ cho người duyệt.

Kind `question` hỏi người một lượt câu hỏi có cấu trúc và bắt buộc có `--questions <file.json>`: mảng đúng 1 câu `{"text", "situation", "options", "recommended", "default_if_silent", "audience"}`. `text` tối đa 300 ký tự; `situation` không trống; `options` có 2 đến 4 phần tử `{"label", "detail", "consequence"}`, `label` tối đa 24 ký tự, cả ba trường không trống; `recommended` bắt buộc có, là chỉ số phương án khuyên chọn; `default_if_silent` là chỉ số phương án tự áp dụng khi không ai trả lời, hoặc `null` để không áp dụng gì; `audience` là `business` hoặc `technical`. Bản ghi của kind này có thêm trường `questions`; kind khác mà có `--questions` bị từ chối với mã 2.

Bản ghi gồm `id`, `kind`, `summary`, `files` (đường dẫn tuyệt đối), `command`, `task` (lấy từ `ZNF_TASK_ID` khi khớp `^[A-Za-z0-9._-]{1,64}$`, không thì để rỗng), `repo`, `branch`, `cwd`, `created_at`, và `questions` (chỉ có ở kind `question`, kind khác không có trường này). Ngoài git repo thì `repo` và `branch` để rỗng.

## Ví dụ

```bash
zenify await question --summary "Cần chốt luật đặt lại đơn" --questions /abs/questions.json   # file: [{"text":"Khách đã huỷ đơn có được đặt lại không?","situation":"Luật hiện tại chưa nói gì về đơn đã huỷ.","options":[{"label":"Cho đặt lại","detail":"Khách bấm đặt lại ngay trên đơn cũ.","consequence":"Đơn cũ mở lại, báo cáo đếm hai lần."},{"label":"Tạo đơn mới","detail":"Khách phải đặt đơn mới.","consequence":"Đơn cũ giữ nguyên trạng thái huỷ."}],"recommended":1,"default_if_silent":null,"audience":"business"}]
zenify await spec --summary "Spec cho tính năng X" --file docs/specs/zenify-kit/x-design.md
zenify await data-write --summary "Backfill field y" --command 'cd /abs/repo && zenify data run --prd -- node y.js --dry-run && zenify data run --prd --write -- node y.js'
```
