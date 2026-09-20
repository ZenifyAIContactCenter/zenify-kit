---
title: Agent
---

# Agent

Danh sách agent chuyên trách của ZenifyKit.

Agent được các skill dispatch qua công cụ Agent, bạn không gọi chúng trực tiếp. Mỗi agent làm đúng một việc hẹp — review code, tìm phụ thuộc, hoặc kiểm giao diện — rồi trả về một báo cáo ngắn cho skill đã gọi nó.

## Danh sách

| Tên | Mô tả |
|---|---|
| [code-reviewer](./code-reviewer) | Reviewer độc lập không có ký ức về việc viết ra diff, kiểm lỗi đúng/sai, bảo mật, hợp đồng, và over-engineering. |
| [researcher](./researcher) | Worker research web với hợp đồng output cố định — mỗi phát hiện là một quote nguyên văn từ URL đã fetch, hoặc ghi rõ không tìm thấy; có mode verify fetch lại URL của worker khác. |
| [scout](./scout) | Agent chỉ đọc trả lời câu hỏi cái gì phụ thuộc vào một thứ sắp thay đổi, không xác minh hình dạng và không review code. |
| [ui-verifier](./ui-verifier) | Agent lái trình duyệt Playwright để kiểm một thay đổi giao diện cả về hành vi lẫn giao diện thật. |
