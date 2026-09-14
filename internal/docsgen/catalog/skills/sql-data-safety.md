---
summary: Đọc/ghi SQL an toàn qua connection pool và SQL viết tay — bộ lọc tenant, tham số hoá, transaction.
---
## Khi nào dùng

Khi đọc hoặc ghi SQL qua một connection pool và câu lệnh SQL viết tay, không phải qua một ORM ánh xạ đầy đủ. Cần cài đặt trước bằng lệnh cài skill coding.

## Cách hoạt động

1. Skill nhắc một bảng đa tenant dùng chung không có row-level security mặc định: mọi câu `SELECT`, `UPDATE`, `DELETE` phải mang bộ lọc tenant ngay trong câu lệnh, không phải kiểm tra sau khi đã lấy dòng về — kiểm tra sau khi lấy dòng vẫn để lộ khoảng hở khi có thay đổi đồng thời.
2. Skill nhấn mạnh tham số hoá mọi giá trị thay đổi theo runtime bằng placeholder, không bao giờ nối chuỗi trực tiếp vào câu SQL — đây là toàn bộ lớp phòng thủ chống SQL injection cho SQL viết tay, không có lớp thứ hai bên dưới.
3. Skill hướng dẫn giới hạn kích thước connection pool và đặt timeout tường minh cho việc lấy connection lẫn cho từng câu truy vấn, vì giá trị mặc định của driver thường là "chờ vô hạn".
4. Skill nêu hai lỗi hay gặp trong transaction: dùng nhầm connection ngoài transaction khiến câu lệnh đó không rollback theo, và không giải phóng connection trong khối `finally` khiến pool cạn dần khi có lỗi xảy ra sớm.
5. Skill nhắc thay đổi schema phải đi qua migration đã review, không để tầng dữ liệu tự đồng bộ schema lúc khởi động, vì việc đó có thể xoá cột và mất dữ liệu khi model trong code và bảng thật lệch nhau.

## Lưu ý

Bất biến bộ lọc tenant giống hệt cho một kho document không schema được nêu ở skill `mongo-data-safety`. An toàn dual-write và tính idempotent cho một thao tác ghi cùng lúc vào SQL và một hệ thống khác thuộc về skill `service-integration`.
