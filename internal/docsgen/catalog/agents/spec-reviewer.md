---
summary: Reviewer spec với context mới, đọc spec đã viết đối chiếu với yêu cầu gốc của bạn trước khi bạn duyệt; chỉ báo yêu cầu thiếu hoặc mâu thuẫn, FR/SC không kiểm được, mâu thuẫn nội bộ, tên hoặc hợp đồng không có nguồn.
---
Agent đọc spec trong một context không tham gia viết nó, nên không mang theo giả định của session đã viết. Brief đưa ba đường dẫn tuyệt đối: spec, file yêu cầu gốc của bạn, và file report agent phải ghi. Toàn bộ finding ghi vào file report; agent chỉ trả về tối đa 40 dòng, dòng đầu là `model: <tên model>` để bạn thấy review chạy trên model nào.

## Kết quả

Mỗi finding có mức `HIGH`, `MEDIUM` hoặc `LOW`, vị trí trong spec, bằng chứng và một câu sửa. `HIGH` nghĩa là làm đúng spec vẫn ra sai thứ bạn cần. Agent không đề xuất mở rộng phạm vi, không góp ý văn phong; spec tốt có thể có ít finding.

## Khi nào được gọi

`znf:brainstorming` tầng architectural dispatch agent một lần ở bước Spec review, sau bước tự review spec và trước khi xin bạn duyệt. Finding `HIGH` được đưa vào lời xin duyệt spec. Agent lỗi hoặc không trả report thì skill ghi `spec review: no report` và vẫn tới bước bạn duyệt.

## Lưu ý

Agent chạy trên `opus` khai trong frontmatter. Máy nào đặt env `ZNF_SPEC_REVIEW_MODEL=fable` thì brainstorming truyền `model: fable` khi dispatch. `Bash` chỉ dùng để đọc code (`git show`, `git grep`, `rg`); agent không ghi gì ngoài file report.
