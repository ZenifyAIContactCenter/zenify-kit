---
summary: Theo dõi CI GitHub Actions của commit vừa push ở chế độ nền và đánh thức phiên khi CI chạy xong; nếu fail, đọc log lỗi, tự sửa trên feature branch, push và theo dõi lại.
---
## Khi nào dùng

Sau khi push một branch, khi bạn muốn Claude tự biết CI xanh hay đỏ mà không phải chép log vào: gõ `/znf:ci-watch`, hoặc nhờ "theo dõi CI branch này".

## Cách hoạt động

1. Skill xác định repo, branch và commit `HEAD`, kiểm commit đã có trên remote. Chưa push thì dừng và báo.
2. Skill chạy một vòng lặp ở chế độ nền: cứ 20 giây gọi `gh run list --commit <sha>` một lần, chờ mọi workflow của commit đó chạy xong. Vòng lặp chỉ in bảng kết quả cuối, mỗi run một dòng.
3. Khi vòng lặp kết thúc, Claude Code đánh thức phiên, kể cả khi phiên đang rảnh.
4. CI xanh: báo một dòng. Có run `failure`, `timed_out` hoặc `startup_failure`: đọc `gh run view <id> --log-failed`, nêu nguyên nhân, rồi tự sửa qua `/znf:fix` (không hỏi), commit, push feature branch và theo dõi lại commit mới.
5. Sau 2 vòng sửa mà CI vẫn đỏ, hoặc nguyên nhân nằm ngoài repo (secret, runner, dịch vụ ngoài, test chập chờn), skill dừng và báo bạn từng vòng đã sửa gì.

## Lưu ý

- Skill chỉ sửa và push trên feature branch. Gặp deploy branch thì chỉ báo nguyên nhân, không sửa.
- Skill không chạy lại, huỷ hay duyệt workflow, không merge, không nới hoặc bỏ test để CI qua.
- Mỗi lần theo dõi gắn với một commit. Push mới thì gọi lại skill cho commit mới.
- Sau 5 phút mà commit không có run nào, skill dừng và báo (thường do repo không có workflow cho branch hoặc event đó). Chờ quá 90 phút thì dừng và báo trạng thái hiện tại.
- Phiên đóng thì vòng lặp nền dừng theo.
