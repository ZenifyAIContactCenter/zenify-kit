---
summary: Engine review hợp nhất của kit, tự chọn mức độ soi theo diff rồi trả về danh sách phát hiện.
---
## Khi nào dùng

Gọi trực tiếp qua `/znf:review` khi muốn review một diff độc lập. Đây cũng là bước review mà `/znf:ship` dùng ở bước cuối trước khi commit và push, nên phần lớn thời gian bạn không gọi skill này bằng tay mà gặp nó qua `/znf:ship`.

## Cách hoạt động

1. Skill đi qua bốn gate: PRE, BUNDLE, REVIEW, VERIFY. Gate PRE chạy build/lint và một lượt quét lỗi mẫu cơ bản trước, không tốn ngân sách LLM nếu bước này đã fail.
2. Gate BUNDLE chỉ chạy với diff rất lớn: diff được tách thành nhiều cụm file nhỏ hơn, mỗi cụm review riêng rồi gộp kết quả; nếu vẫn quá lớn để tách, skill dừng và đề nghị chia nhỏ pull request.
3. Gate REVIEW đo diff để chọn mức độ soi: diff nhỏ dùng một reviewer độc lập (T1); diff vừa dùng năm reviewer chạy song song (T2), mỗi người phụ trách một trục: lỗi, bảo mật, hiệu năng, hợp đồng, và ý định (`intent`); diff lớn hoặc chạm vùng nhạy cảm (auth, tenant, migration) dùng T3, nghĩa là năm trục như T2, cả năm reviewer chạy opus, không còn Workflow hay vòng skeptic. Diff chạm hợp đồng chung (một collection, endpoint, queue, hoặc kênh pub/sub) được nâng mức soi tối thiểu lên mức vừa.
4. Trục `intent` so diff với phần `## Intent` trong gói ship; chạy độc lập, không có gói thì so với mục tiêu bạn đã nêu. Mỗi reviewer còn đọc `CLAUDE.md`/`AGENTS.md` của repo và các file `.claude/rules/` khớp với diff trước khi chấm, và trích `file:dòng` của quy ước bị vi phạm.
5. Gate VERIFY kiểm cơ học: mọi phát hiện có kèm vị trí file phải trích đúng dòng code thật; skill loại bỏ phát hiện nào trích sai.
6. Kết quả cuối gồm mức soi kèm lý do, danh sách phát hiện xếp theo mức độ nghiêm trọng và một câu trả lời có thể ship được hay không.

## Lưu ý

Không có diff nào để review, hoặc không phải một git repo, skill báo "nothing to review" và dừng, không dispatch reviewer nào.
