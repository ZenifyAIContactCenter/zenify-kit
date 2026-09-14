---
summary: Kiểm tra cơ học một cặp spec và plan trước khi code: coverage FR, marker còn sót, cấu trúc Brief.
---
## Khi nào dùng

Sau khi viết xong spec và plan, trước khi bắt đầu implement. Skill `/znf:analyze` gọi lệnh này rồi bổ sung phần nhận định. Bạn có thể chạy tay để xem phần cơ học.

## Kết quả

Lệnh đọc hai file markdown và in ba mục: Brief có mặt hay không và số trường đã điền, số FR trong spec so với số FR được plan nhắc tới, và danh sách finding. Finding gồm FR không có task, task không trỏ về FR, tham chiếu hỏng, marker `[NEEDS CLARIFICATION` còn sót, và ba tag rủi ro thiếu trong Brief.

Lệnh chỉ cảnh báo, không chặn. Khi không đọc được file, nó in lý do và kết thúc bình thường.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--spec` | Đường dẫn file spec. |
| `--plan` | Đường dẫn file plan. |
| `--json` | In kết quả dạng JSON thay cho bản tóm tắt. |

## Ví dụ

```bash
zenify analyze --spec docs/specs/zenify-kit/2026-09-14-kit-docs-site-design.md \
  --plan docs/plans/zenify-kit/2026-09-14-kit-docs-site.md
```
