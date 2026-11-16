---
summary: Ghi nhận hạ tầng sandbox hỏng nên không thể kiểm UI (chỉ chạy được khi ZNF_AUTONOMOUS=1).
---
## Khi nào dùng

Worker autonomous (sandbox, không có người) gọi lệnh này khi chính hạ tầng hỏng khiến không đo được UI: deps không cài được, app không khởi động. Không dùng cho lỗi UI thật, và không dùng ngoài autonomous: lệnh từ chối và thoát khác 0.

## Kết quả

Lệnh ghi `<repo>/.znf/ui-verify/<fp>.blocked.json` (`{"fp","reason","recorded_at"}`) cho fingerprint hiện tại, rồi in `fp`. Thư mục này tự ignore nên ghi không đổi fingerprint. `zenify ui-verify check` sau đó báo `infra-blocked: <mã>` và thoát 0, không tạo await, để PR vẫn mở với dòng cảnh báo. Artifact hợp lệ luôn thắng bản ghi blocked; sửa thêm file sau khi ghi làm fingerprint lệch nên bản ghi cũ không còn tính.

Nếu env `ZNF_UI_STATUS_FILE` khác rỗng, lệnh ghi thêm file đó với `{"state":"infra-blocked","reason":"<mã>"}` để host cảnh báo admin. Ghi file này lỗi chỉ in cảnh báo ra stderr, lệnh vẫn thoát 0.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--repo` | Đường dẫn repo. Mặc định thư mục hiện tại. |
| `--reason` | Mã lý do, vd `deps_auth_failed:contact-center-hub`. Bắt buộc, không được rỗng. |

## Ví dụ

```bash
ZNF_AUTONOMOUS=1 zenify ui-verify blocked --repo repos/contact-center-web \
  --reason deps_auth_failed:contact-center-web
```
