---
summary: Research có kiểm chứng: worker tìm và trích dẫn nguyên văn kèm URL đã fetch, một lượt haiku fetch lại mọi URL, kết quả ghi thành file trong docs/reference.
---
## Khi nào dùng

Khi thiết kế phụ thuộc vào thư viện, framework, tool hay API ngoài mà session chưa đọc docs; khi cần biết người khác giải bài toán này thế nào (prior art, thực hành cộng đồng, sản phẩm đối thủ); khi có sự thật dễ lỗi thời (latest, version, giá, release note); hoặc khi có hai phương án thật sự khác nhau mà chọn sai tốn hơn một lượt research. Không dùng cho tên file, field, collection hay endpoint trong repo — đó là việc của `/znf:ground` và `/znf:scout`.

Skill chạy tách khỏi phiên chính (`context: fork`): phiên chính chỉ nhận tối đa 40 dòng và một đường dẫn file.

## Cách hoạt động

1. **Scope.** Viết lại câu hỏi và tiêu chí "trả lời xong là gì"; tách câu hỏi con theo ranh giới nguồn (một họ tài liệu, một hệ thống cho mỗi worker); chọn cỡ 1 / 2–4 / 5+ worker và cap số lần gọi tool. Khối scope này mở đầu báo cáo và phần trả về, thay cho bước duyệt kế hoạch — skill chạy fork nên không hỏi được.
2. **Dispatch.** Mỗi câu hỏi con một agent `znf:researcher` (sonnet), gửi trong một tin nhắn. Worker ghi toàn bộ vào file tạm và chỉ trả về vài dòng đếm. Worker im lặng được dispatch lại một lần, rồi ghi là khoảng trống.
3. **Verify.** Một `znf:researcher` mode `verify` (haiku) fetch lại mọi URL, so quote với trang, gắn cờ `VERIFIED` / `QUOTE-MISMATCH` / `DEAD-URL` / `NOT-FETCHED`.
4. **Tổng hợp.** Lead đọc file worker và file verify, viết `docs/reference/<repo>/YYYY-MM-DD-<slug>.md`: câu hỏi, cách làm, bảng claim kèm URL và trạng thái verify, mục "không tìm thấy", kết luận chỉ từ hàng đã verify.
5. **Trả về.** Đường dẫn file, khối scope, số đếm verified / flagged / not found, kết luận tối đa 5 dòng.

## Được gọi từ đâu

Gọi tay `/znf:research <câu hỏi> repo: <repo>`, hoặc tự động từ `/znf:brainstorming` khi checklist năm mục ở bước Research check có một mục đúng. Khi vào từ brainstorming, spec trỏ tới file kết quả ở trường Approach và không đưa claim bị gắn cờ vào spec.

## Lưu ý

Mọi claim không có URL đã fetch trong lượt chạy là không hợp lệ — worker không được trích từ trí nhớ. Skill không commit, không chạy git trong knowledge store (kit tự sync). Chi phí một lượt so sánh ba worker khoảng 5–6 USD; xem bảng cỡ trong reference của skill.
