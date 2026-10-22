---
title: Chế độ không người
---

# Chế độ không người

Chế độ này dành cho worker chạy nền (không có người ngồi trả lời). Khi `ZNF_AUTONOMOUS=1` (đúng chuỗi `1`), agent không hỏi người ở bất cứ cổng nào của skill `znf:*`. Nó tự quyết, ghi lại quyết định, và chỉ dừng ở năm điểm cố định bằng `zenify await`. Mọi thứ khác trong skill vẫn áp dụng như thường.

## Bật chế độ

Đặt biến môi trường trước khi chạy `claude`:

```sh
export ZNF_AUTONOMOUS=1
export ZNF_TASK_ID=task-42   # tuỳ chọn: id task, ghi vào bản ghi await và audit của broker
```

Khi biến này bật, dòng đầu session có thêm một dòng nhắc agent đọc `~/.claude/skills/znf/skills/_shared/autonomous.md`; file đó là luật agent theo. Worker nên chạy với role `agent` của broker, xem [Dùng credential broker](/guides/broker-client#role-agent): đọc được staging và production, ghi được staging, không ghi được production.

## Agent tự quyết, không hỏi

Chỗ nào skill định hỏi hay chờ người, agent tự quyết và ghi một dòng `Ruling: <quyết định> — <lý do> — <cái giá nếu sai>` vào ledger (`progress.md` của SDD) hoặc, ngoài SDD, vào mục `## Intent` của ship pack. Riêng pull request: agent không bao giờ merge, kể cả PR do chính nó mở.

## Năm điểm dừng

| `kind` | Khi nào |
|---|---|
| `spec` | spec của `brainstorming` đã xong, cần người duyệt (cổng thiết kế của `cook` gộp vào đây) |
| `plan` | plan của `cook` cần người duyệt trước khi chạy SDD |
| `data-write` | script ghi dữ liệu production đã dry-run, chờ lead chạy `--write` |
| `irreversible` | việc không hoàn tác được, nhạy cảm về bảo mật, hoặc có tác dụng ngoài worktree |
| `needs-human` | vòng sửa của `ship` vẫn còn lỗi sau vòng 2, hoặc plan hỏng đến mức không làm tiếp được |

## `zenify await`

```sh
zenify await <spec|plan|data-write|irreversible|needs-human> --summary "<điều người cần quyết>" [--file <đường dẫn>]… [--command "<lệnh>"]
```

Lệnh ghi một bản ghi JSON vào `$ZENIFY_HOME/await/<id>.json` (mặc định `~/.zenify/await/`), in `id` ra stdout rồi thoát với **mã 8**. Mã 8 là điểm dừng đúng kế hoạch, không phải lỗi: agent kết thúc session ngay, không thử lại. Host của worker nhặt bản ghi trong thư mục đó. `--file` lặp được, mỗi file phải tồn tại và được lưu thành đường dẫn tuyệt đối; `--command` dùng cho `data-write`.

Ví dụ thật (chạy với `ZENIFY_HOME` tạm):

```text
$ zenify await spec --summary x
error: zenify await only works in autonomous mode (ZNF_AUTONOMOUS=1)      # thoát mã 2

$ ZNF_AUTONOMOUS=1 ZNF_TASK_ID=task-42 zenify await spec --summary "Duyệt spec đăng nhập" --file go.mod
spec-20261022T030837Z-68f708ec
awaiting human: spec (spec-20261022T030837Z-68f708ec)                      # thoát mã 8

$ ZNF_AUTONOMOUS=1 zenify await foo
error: unknown kind "foo", want one of: spec, plan, data-write, irreversible, needs-human   # thoát mã 2
```

Xem lại một bản ghi bằng `zenify await show <id>`:

```text
$ zenify await show spec-20261022T030837Z-68f708ec
{
  "id": "spec-20261022T030837Z-68f708ec",
  "kind": "spec",
  "summary": "Duyệt spec đăng nhập",
  "files": [
    "/…/zenify-kit/go.mod"
  ],
  "command": "",
  "task": "task-42",
  "repo": "/…/zenify-kit",
  "branch": "<branch hiện tại>",
  "cwd": "/…/zenify-kit",
  "created_at": "2026-10-22T03:08:37.175366Z"
}
```

`task` lấy từ `ZNF_TASK_ID`; `repo` và `branch` để trống khi lệnh chạy ngoài git repo. `id` chỉ là tên file trần: `await show` từ chối id có dấu `/` hoặc bắt đầu bằng `.` (mã 2), và báo `no await record "<id>"` khi không có bản ghi.

## `data-write`: ghi production {#data-write}

Role `agent` không bao giờ lấy được credential ghi production. Quy trình: agent dry-run (`zenify data run --prd -- …`), rồi chạy `zenify await data-write --summary "<script sửa gì>" --command "zenify data run --prd --write -- …"`, với `--command` là đúng lệnh đã dry-run. Duyệt nghĩa là một lead chạy lệnh đó; worker không resume để ghi. Nếu agent lỡ chạy thẳng `zenify data run --prd --write` ở role `agent` (sau khi đã dry-run, vì thứ tự kiểm không đổi), lệnh thoát mã 1 và in sẵn lệnh `zenify await data-write` cần chạy. Script ghi staging (`--stg --write`) không dừng: nó chạy ngay sau dry-run như [`data run`](/guides/broker-client#data-run) mô tả.

## Resume

Worker được khởi động lại với prompt `resume <id> approved` hoặc `resume <id> rejected: <lý do>`. Agent chạy `zenify await show <id>` trước, rồi:

- `approved`: làm tiếp từ điểm đã dừng.
- `rejected`: sửa điều lý do nêu, rồi `zenify await` lại ở đúng cổng đó.

## Xem thêm

[`zenify await`](/reference/cli/zenify_await), [`zenify await show`](/reference/cli/zenify_await_show), [Dùng credential broker](/guides/broker-client), [Dựng host credential broker](/guides/broker-host#role-agent).

<!-- Nguồn (cho người bảo trì, không hiển thị):
- `internal/cli/await.go` (kind, record, mã 8, ZNF_AUTONOMOUS/ZNF_TASK_ID)
- `internal/plugin/assets/znf/skills/_shared/autonomous.md` (luật, bảng cổng, resume)
- `internal/cli/hookactions.go` autonomousLine (dòng SessionStart)
- `internal/cli/data.go` (data run --prd --write với role agent)
-->
