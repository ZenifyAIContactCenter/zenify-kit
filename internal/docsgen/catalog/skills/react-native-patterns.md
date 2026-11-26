---
summary: Quy ước viết app React Native — token màu/chữ thay mã hex, đổi JS hay đổi native, module native iOS/Android đối xứng, quyền, push nền, log bản release.
---
## Khi nào dùng

Khi viết hoặc sửa code trong một app React Native (repo có dependency `react-native` và thư mục `ios/`, `android/`). Bảng định tuyến `_shared/skill-routing` tự gợi ý skill này cho task chạm vào app; gọi tay bằng `/znf:react-native-patterns`. Skill `<repo>-conventions` của app (nếu có) ghi tên cụ thể của module theme, bộ UI và module native — đọc cả hai, chỗ khác nhau thì theo skill của repo.

## Cách hoạt động

1. Skill yêu cầu màu và kiểu chữ lấy từ một module theme, đặt tên token theo vai trò (`text.secondary`, `bg.subtle`) thay vì theo màu, để sau này thêm dark mode chỉ là thêm một bảng giá trị chứ không phải sửa từng màn.
2. Skill hướng dẫn chuyển app cũ sang token mà không đổi giao diện: chỉ thay mã hex khớp chính xác một token, giữ hằng số cũ (đánh dấu deprecated) thay vì sửa giá trị, và coi mọi thay đổi giá trị là thay đổi thiết kế cần ảnh trước/sau. Chặn tái phát bằng luật lint và một script đếm mà PR chỉ được làm giảm.
3. Skill phân biệt thay đổi chỉ JS (Metro tải lại, ship được qua OTA) với thay đổi native (module native, `Info.plist`, `AndroidManifest.xml`, thư viện có code native) — loại sau phải build lại cả hai nền tảng, không ship qua OTA được. Với thay đổi JS, build thử bundle để bắt lỗi import và alias mà type check không thấy.
4. Skill nhắc một API native phải có ở cả iOS lẫn Android với cùng tên trường sự kiện, và callback có thể về trên luồng nền phải chuyển sang luồng chính trước khi chạm UI.
5. Skill mô tả luồng xin quyền: hỏi đúng lúc cần, `check` → `request` → mở Cài đặt nếu bị chặn; hộp thoại hệ thống chỉ hiện khi app ở foreground nên việc phát sinh lúc ở nền phải đợi app mở lại mới hỏi.

## Lưu ý

Push VoIP và âm thanh cuộc gọi không chạy trên iOS simulator — luồng push / cuộc gọi phải test trên máy thật. Bản release thường bỏ `console.*`, nên log chẩn đoán production phải đi qua logger gửi về server. Phần tối ưu hiệu năng sâu (Hermes, bundle, re-render) để cho skill hiệu năng React Native chuyên dụng nếu máy có cài.
