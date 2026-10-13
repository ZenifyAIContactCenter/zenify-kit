---
summary: Rút những bài học tốn kém của phiên về đúng một nơi (check, rule, CLAUDE.md hoặc memory), đưa một danh sách để bạn duyệt trước khi ghi.
---
## Khi nào dùng

Cuối một phiên vừa có chỗ phải làm lại, phải đọc file hay chạy lệnh mới biết, hoặc bị bạn sửa lại, và bạn muốn bài học đó không mất ở phiên sau.

## Cách hoạt động

1. Skill đọc hội thoại trong context. Trên Claude Code, nếu phiên đã bị nén, skill đọc thêm file transcript của phiên dưới `~/.claude/projects/<slug>/`. Harness khác chỉ dùng hội thoại trong context.
2. Skill chỉ giữ khoảnh khắc thoả một trong hai điều kiện: phải đọc, query hay chạy lệnh mới biết, hoặc bạn đã sửa lại. Mỗi khoảnh khắc kèm trích dẫn nguyên văn, và trích dẫn chỉ nằm trong chat.
3. Mỗi bài học về đúng một nơi: kiểm được bằng máy thì thành check, cần phán đoán thì thành rule, điều hướng thì thành một dòng trong CLAUDE.md, gotcha đo được thì thành memory. Đường dẫn lấy từ cấu hình của project và harness.
4. Trước khi đề xuất, skill đọc memory và rule đã có cùng chủ đề. Trùng thì đề xuất sửa bản cũ, mâu thuẫn thì đề xuất sửa cho đúng, không tạo bản thứ hai.
5. Skill đưa một danh sách duy nhất để bạn chọn mục, không hỏi từng mục một.
6. Skill chỉ ghi các mục bạn chọn. Nội dung ghi là quy tắc, không phải sự kiện, không chứa trích dẫn transcript, và secret (token, mật khẩu, connection string) bị lọc trước khi ghi.
7. Không có khoảnh khắc nào đạt điều kiện thì skill báo không có bài học và dừng, không ghi gì.

## Lưu ý

Chỉ bạn gõ `/znf:retro` mới chạy được skill này, Claude không tự gọi. Với bài học loại check, skill không ghi file và không sửa code, chỉ in trong chat việc cần làm (check nào, ở repo nào, gọi `/znf:cook` hay `/znf:fix`) để bạn tự mở.
