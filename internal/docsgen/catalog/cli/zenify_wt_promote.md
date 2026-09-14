---
summary: Đổi thư mục node_modules dạng symlink của một worktree thành bản copy riêng.
---
## Khi nào dùng

Repo khai `deps: symlink` cho worktree dùng chung `node_modules` với checkout chính. Khi task cần thêm hoặc đổi dependency mà không ảnh hưởng checkout chính, promote worktree đó trước rồi mới cài.

## Kết quả

Lệnh thay symlink bằng bản copy copy-on-write, ghi lại `deps=clone` cho worktree và chạy cài đặt. Ba trường hợp khác nhau:

- Worktree đã có `node_modules` riêng: không làm gì.
- Worktree không có `node_modules` nào: từ chối và nhắc cài trước.
- Repo khai `deps: none`: in thông báo không có gì để promote.

## Ví dụ

```bash
zenify wt promote session-timeout
```

## Lưu ý

Nếu copy xong nhưng không ghi được bookkeeping, `zenify wt ls` hiện cột `DEPS` là `-` và lệnh in dòng `git config` để bạn chạy tay.
