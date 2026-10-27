# Scenario template — `.znf/e2e/scenarios/ZEN-<n>.md`

The section keywords are Vietnamese on purpose: the item owner reads them, and `zenify e2e lint`
parses them by these exact words. Copy the skeleton, keep every keyword spelled exactly as shown.

## Syntax

- Line 1: `# ZEN-<n>: <title>` — the item key, digits only.
- `Vai: <role>` — the role that logs in, in plain words (a role name, not an account); the QA run maps it to a broker E2E role.
- One section per acceptance line: `## @ac:<n> [<kind>] <acceptance sentence, verbatim>`.
  `<n>` is unique; `<kind>` is exactly `e2e`, `hỏi` or `ngoài-e2e`.
- Under an `e2e` section: `Bước:` then numbered steps (`1.`); `Thấy:` then `-` bullets, each the
  exact text visible on screen; optional `Dữ liệu sau:` then `-` bullets for the data outcome.
  An acceptance sentence with several conditions repeats `Bước:`/`Thấy:` once per condition under
  the same heading (`@ac:4` in the worked example); the journey has one test per pair.
- A `hỏi` section holds the plain-language question; a `ngoài-e2e` section says how it is checked
  (ui-verifier or unit) and why it is not a journey.
- Optional last section `## Câu hỏi mở` for open questions. Any other `@ac` heading (another `#` level, another case, another shape) is a parse error.

The skeleton below is a shape, not a valid file: its `<placeholders>` (starting with `ZEN-<n>`)
do not parse until replaced. The worked example after it is valid as written.

```text
# ZEN-<n>: <title>
Vai: <role>

## @ac:1 [e2e] <acceptance sentence>
Bước:
1. <action, using on-screen words>
Thấy:
- <exact on-screen text>
Dữ liệu sau:
- <data outcome>

## @ac:2 [hỏi] <acceptance sentence>
<question for the item owner>

## @ac:3 [ngoài-e2e] <acceptance sentence>
<how it is checked: ui-verifier | unit — and why>

## Câu hỏi mở
- <question>
```

## Worked example

```markdown
# ZEN-123: Tạo ticket thiếu tiêu đề phải báo lỗi
Vai: nhân viên

## @ac:1 [e2e] Khi tạo ticket mà để trống tiêu đề thì hệ thống báo lỗi và không tạo ticket
Bước:
1. Mở màn Ticket, bấm Tạo ticket
2. Để trống ô Tiêu đề
3. Bấm Tạo
Thấy:
- Vui lòng nhập tiêu đề
Dữ liệu sau:
- Không có ticket mới nào được tạo

## @ac:2 [hỏi] Ô tiêu đề được viền đỏ khi thiếu
Ô tiêu đề có cần viền đỏ ngay khi bấm Tạo, hay chỉ cần dòng báo lỗi bên dưới?

## @ac:3 [ngoài-e2e] Thông báo lỗi hiển thị đúng font chữ của hệ thống
Cách kiểm: ui-verifier; đây là kiểm tra giao diện, không đổi dữ liệu.

## @ac:4 [e2e] Khi tiêu đề dài quá 200 ký tự thì báo lỗi; khi tiêu đề có đúng 200 ký tự thì tạo được ticket
Bước:
1. Mở màn Ticket, bấm Tạo ticket
2. Nhập tiêu đề 201 ký tự, bấm Tạo
Thấy:
- Tiêu đề tối đa 200 ký tự
Bước:
1. Mở màn Ticket, bấm Tạo ticket
2. Nhập tiêu đề đúng 200 ký tự, bấm Tạo
Thấy:
- Đã tạo ticket

## Câu hỏi mở
- Tiêu đề chỉ có dấu cách có tính là trống không?
```
