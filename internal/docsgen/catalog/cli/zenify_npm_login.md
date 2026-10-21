---
summary: Đặt NPM_TOKEN từ gh vào shell profile và cấp scope read:packages để npm install được package private.
---
## Khi nào dùng

Khi `zenify doctor` báo mục `npm token (GitHub Packages)` hỏng, hoặc khi `npm install` trả 401 với package trên `npm.pkg.github.com`. Khác `zenify up`, lệnh này luôn chạy, không cần repo nào trong workspace dùng GitHub Packages, và chạy được cả ngoài workspace. Không cần sudo.

## Kết quả

Lệnh xét account github.com đang active của `gh`, rồi ghi một dòng vào profile của shell: zsh vào `~/.zshrc`, bash vào `~/.bashrc` (chưa có thì `~/.profile`).

```bash
export NPM_TOKEN="${NPM_TOKEN:-$(gh auth token 2>/dev/null)}" # zenify-npm-token
```

Token không bị ghi ra đĩa hay in ra. Dòng chỉ gọi `gh auth token` mỗi khi mở shell mới, và `NPM_TOKEN` đã có sẵn trong môi trường thì được giữ nguyên.

- Profile đã tự đặt `NPM_TOKEN` thì lệnh không ghi gì thêm.
- Có terminal tương tác mà `gh` thiếu scope `read:packages` thì lệnh chạy `gh auth refresh -h github.com -s read:packages`. Không có terminal thì in lệnh đó để bạn tự chạy.
- Shell không phải zsh hay bash thì lệnh in dòng `export` ở trên để bạn tự thêm vào profile.
- Cuối cùng lệnh kiểm tra lại như mục `npm token (GitHub Packages)` của `zenify doctor` và in `npm token: ok`. Còn lỗi thì thoát với mã khác 0 kèm chi tiết.

## Ví dụ

```bash
zenify npm login
source ~/.zshrc
```

## Lưu ý

Sau khi lệnh ghi profile, chạy `source <profile>` hoặc mở terminal mới trước khi `npm install`. Đây là toàn bộ token của `gh auth token` (mọi scope `gh` đang giữ), được export cho mọi shell mới.

Gỡ: xoá dòng có marker `# zenify-npm-token` khỏi profile. Muốn bỏ cả scope thì chạy `gh auth refresh -h github.com --remove-scopes read:packages`.
