# Finding schema — dùng chung cho mọi tier của znf:review

Mọi tier (T1/T2/T3) và mọi capability (M4b finding-verifier, M4e learning-capture)
trả finding theo CÙNG một shape. Đây là nguồn schema duy nhất — không định nghĩa lại nơi khác.

## Finding

- `dimension` (string): `bugs | security | perf | contracts | types`
- `severity` (string): `CRITICAL | HIGH | MEDIUM | LOW`
- `title` (string): nhãn ngắn
- `file` (string): path repo-relative
- `line` (string): dòng (hoặc range) finding neo vào
- `issue` (string): một câu mô tả lỗi
- `fix` (string): cách sửa đề xuất
- `evidence` (string): trích **verbatim** MỘT dòng code mà finding trỏ tới (nguyên văn nội dung dòng trong file, KHÔNG kèm dấu `+`/`-` của diff — verifier so khớp với file thật). **Bắt buộc khi có `file+line`**.

Bắt buộc: `dimension, severity, title, issue, fix`. `file/line` khuyến nghị khi có vị trí; `evidence` bắt buộc khi có `file+line` (để `zenify review-verify` kiểm chứng citation).

## Verdict (adversarial verify — T3, và M4b finding-verifier về sau)

- `refuted` (bool): finding bị bác?
- `reason` (string): lý do

Chỉ finding KHÔNG bị bác (đủ số skeptic confirm ở T3) mới vào report.
