---
summary: Các hook ZenifyKit nối vào Claude Code để tự động đồng bộ, báo cáo trạng thái, và đo mức dùng.
---
`zenify up` viết các hook dưới đây vào `<workspace>/.claude/settings.local.json`, nên chúng chỉ chạy khi session mở ở gốc workspace; `~/.claude/settings.json` không còn hook nào của kit. Mỗi hook chỉ là một lệnh dạng `zenify hooks-run <id>`, nên gỡ hay đọc lại cũng chỉ là một dòng cấu hình. Mọi hook đều fail-open: lỗi bên trong không chặn phiên làm việc của bạn. Các hook chỉ hoạt động khi phiên chạy bên trong một workspace ZenifyKit đã khởi tạo.
