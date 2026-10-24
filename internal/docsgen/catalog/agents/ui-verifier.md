---
summary: Agent lái trình duyệt Playwright để kiểm một thay đổi giao diện cả về hành vi lẫn giao diện thật.
---
Agent chỉ quan sát và báo cáo, không tự sửa code. Nó lái trình duyệt qua đúng luồng thay đổi chạm tới, rồi đo bằng số đo thật (kích thước và vị trí phần tử so với khung chứa của nó) thay vì chỉ nhìn qua ảnh chụp — một thay đổi có thể vượt qua mọi kiểm tra hành vi trong khi giao diện vẫn vỡ bố cục.

## Khi nào được gọi

`/znf:ship` dispatch agent này ở bước xác minh hành vi cuối cùng khi thay đổi có render ra màn hình, và đây là nơi duy nhất trong pipeline gọi nó. `/znf:cook` gọi qua bước implement khi một task được plan đánh dấu cần kiểm giao diện.

## Kết quả trả về

Một câu trả lời PASS, FAIL, BLOCKED, hoặc PARTIAL cho từng phần đã kiểm, kèm bằng chứng hành vi đã lái qua, số đo bố cục cụ thể, và số lỗi console mới so với lỗi đã có từ trước. Không dán nguyên cây DOM hay toàn bộ log console.

## Lưu ý

Agent lái trình duyệt bằng `zenify browser -s=<phiên>`, mỗi verifier một phiên riêng với trình duyệt và thư mục riêng, nên nhiều verifier chạy song song được; agent chỉ dùng phiên của mình và luôn `close` phiên đó khi xong. Agent tự đăng nhập: chạy `zenify e2e login --url <app>` (tài khoản lấy từ credential broker; manifest không có broker thì đọc `E2E_DOMAIN`, `E2E_EMAIL`, `E2E_PASSWORD` từ môi trường), mở phiên (`open`), nạp `~/.zenify/playwright/state.json` bằng `state-load`, rồi mới điều hướng. Khi người gọi nêu role cần kiểm, agent thêm `--role <role>` và `state-load` file `~/.zenify/playwright/state-<role>.json`. Phiên chính không đăng nhập trình duyệt. Nếu login thất bại hoặc thiếu biến, agent báo BLOCKED thay vì đoán.
