---
summary: Engine review hợp nhất của kit, tự chọn mức độ soi theo diff rồi trả về danh sách phát hiện.
---
## Khi nào dùng

Gọi trực tiếp qua `/znf:review` khi muốn review một diff độc lập. Đây cũng là bước review mà `/znf:ship` dùng ở bước cuối trước khi commit và push, nên phần lớn thời gian bạn không gọi skill này bằng tay mà gặp nó qua `/znf:ship`.

## Cách hoạt động

1. Skill chạy build/lint và một lượt quét lỗi mẫu cơ bản trước, không tốn ngân sách LLM nếu bước này đã fail.
2. Skill đo diff để chọn mức độ soi: diff nhỏ dùng một reviewer độc lập; diff vừa dùng năm reviewer chạy song song, mỗi người phụ trách một khía cạnh (lỗi, bảo mật, hiệu năng, hợp đồng, kiểu dữ liệu); diff lớn hoặc chạm vùng nhạy cảm (auth, tenant, migration) dùng một vòng review đối kháng, nơi các phát hiện nghiêm trọng phải được nhiều reviewer độc lập xác nhận mới giữ lại. Diff chạm hợp đồng chung (một collection, endpoint, queue, hoặc kênh pub/sub) được nâng mức soi tối thiểu lên mức vừa.
3. Mọi phát hiện có kèm vị trí file phải trích đúng dòng code thật; skill xác minh cơ học từng trích dẫn đó khớp với file thật và loại bỏ phát hiện nào trích sai.
4. Diff rất lớn được tách thành nhiều cụm file nhỏ hơn, mỗi cụm review riêng rồi gộp kết quả; nếu vẫn quá lớn để tách, skill dừng và đề nghị chia nhỏ pull request.
5. Kết quả cuối gồm danh sách phát hiện xếp theo mức độ nghiêm trọng và một câu trả lời có thể ship được hay không. Đôi khi skill kèm thêm một vài ghi chú tư vấn chỉ mang tính tham khảo, không ảnh hưởng tới câu trả lời có ship được hay không.

## Lưu ý

Không có diff nào để review, hoặc không phải một git repo, skill báo "nothing to review" và dừng, không dispatch reviewer nào.
