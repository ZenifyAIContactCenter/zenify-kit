---
summary: Quản lý ledger track bằng mã ngắn từ một lệnh duy nhất: thêm, bỏ, xem Plan và Backlog, lập plan, bắt đầu, kết thúc hoặc huỷ việc, đặt estimate.
---
## Khi nào dùng

Khi bạn muốn thao tác với Plan và Backlog của pane track mà không gõ lệnh `zenify track` bằng tay: gõ `/znf:track` kèm một verb, hoặc nói "track", "backlog", "plan today", "next task".

## Cách hoạt động

1. Verb đầu tiên của args quyết định việc làm: `add`, `drop`, `ls`, `plan`, `plan +`, `plan -`, `go`, `done`, `cancel`, `est`, `help`. Không có verb hoặc verb lạ thì skill in bảng help và không chạy lệnh ghi nào.
2. Mã ngắn (`b81e`, `#581`, `<repo>#581`, `ZEN-288`) được truyền nguyên cho CLI; CLI tự resolve. Mã trùng hoặc không khớp thì CLI thoát lỗi kèm danh sách ứng viên và skill đưa nguyên thông báo cho bạn chọn.
3. `done`, `cancel`, `est` làm việc trên id đang mở của session; không còn id trong hội thoại thì skill đọc các item `running` của `zenify track stats --brief`: đúng một item thì dùng, từ hai item thì hỏi bạn, không có thì báo và không đoán.
4. `go <mã>` gọi `znf:cook` hoặc `znf:fix` cho việc có code, hoặc `zenify track start --kind task` cho việc ops không có PR.

## Lưu ý

Skill chỉ ghi qua CLI `zenify track`, không sửa trực tiếp `.zenify/track/`. `add --plane` cần lệnh `zenify plane create`, chưa có cho tới PR2. Chi tiết mã ngắn và các màn hình pane: xem [Pane track](/guides/track-pane).
