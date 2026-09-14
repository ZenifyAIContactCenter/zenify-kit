---
summary: Agent lái trình duyệt Playwright để kiểm một thay đổi giao diện cả về hành vi lẫn giao diện thật.
---
Agent chỉ quan sát và báo cáo, không tự sửa code. Nó lái trình duyệt qua đúng luồng thay đổi chạm tới, rồi đo bằng số đo thật (kích thước và vị trí phần tử so với khung chứa của nó) thay vì chỉ nhìn qua ảnh chụp — một thay đổi có thể vượt qua mọi kiểm tra hành vi trong khi giao diện vẫn vỡ bố cục.

## Khi nào được gọi

`/znf:ship` dispatch agent này ở bước xác minh hành vi cuối cùng khi thay đổi có render ra màn hình, và đây là nơi duy nhất trong pipeline gọi nó. `/znf:cook` gọi qua bước implement khi một task được plan đánh dấu cần kiểm giao diện.

## Kết quả trả về

Một câu trả lời PASS, FAIL, BLOCKED, hoặc PARTIAL cho từng phần đã kiểm, kèm bằng chứng hành vi đã lái qua, số đo bố cục cụ thể, và số lỗi console mới so với lỗi đã có từ trước. Không dán nguyên cây DOM hay toàn bộ log console.

## Lưu ý

Trình duyệt Playwright là một instance dùng chung duy nhất, nên không được chạy hai agent lái trình duyệt cùng lúc, và phiên chính không được tự thao tác Playwright trong lúc agent này đang chạy. Nếu app cần đăng nhập mà không có thông tin đăng nhập, agent báo BLOCKED thay vì đoán.
