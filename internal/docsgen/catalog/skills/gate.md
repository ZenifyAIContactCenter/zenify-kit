---
summary: Cổng kiểm hợp đồng chung giữa các repo trong một polyrepo, chạy ngay sau khi sửa thứ gì đó dùng chung.
---
## Khi nào dùng

Ngay sau khi sửa một collection/bảng dùng chung, một kênh pub/sub, một endpoint giữa các dịch vụ, hoặc một queue. Chỉ đọc, an toàn để chạy tự do. Có thể gọi bằng tay với tên tài nguyên cụ thể. Xem [Base ref và cổng](/concepts/gate-fail-open) và trang [ship: verify và mở PR](/workflows/ship), nơi gate được gọi tự động.

## Cách hoạt động

1. Skill lấy danh sách repo thực sự tham gia hợp đồng chung từ cấu hình của workspace, không đoán theo cảm giác "trông có vẻ liên quan".
2. Với mỗi repo, skill tìm nơi khai báo tài nguyên, mọi nơi đọc/ghi qua lớp truy cập của repo đó, và mọi nơi truy cập trực tiếp bỏ qua lớp đó — ba lượt tìm khác nhau, vì chỉ grep tên tài nguyên sẽ bỏ sót gần hết.
3. Skill dispatch việc tìm kiếm ở mỗi repo song song, mỗi repo một agent, rồi tổng hợp thành bảng usage kèm nhận định phá vỡ hay không.
4. Với tài nguyên là DB, skill xác minh tên collection/bảng và tên field bằng dữ liệu thật thay vì tin vào tên trong code hay tài liệu.
5. Kết quả in ra thứ tự triển khai an toàn khi thay đổi phá vỡ hợp đồng, ví dụ: schema/migration trước, rồi backend, rồi subscriber trước publisher, rồi frontend sau cùng.

## Ví dụ

```text
/znf:gate
/znf:gate chat_rooms
```

## Lưu ý

Khi số lượng usage tìm được quá nhiều để đánh giá từng cái bằng một lượt kiểm đơn giản, hoặc thay đổi chạm nhiều tài nguyên chung cùng lúc, có một workflow riêng nặng hơn để quét sâu hơn — dùng khi gate đơn không đủ tin cậy.
