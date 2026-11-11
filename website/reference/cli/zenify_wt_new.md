---
title: zenify wt new
---

[Lệnh CLI](/reference/cli/) / `zenify wt new`

# `zenify wt new`

Tạo worktree mới cho một task, kèm branch, port riêng và môi trường dev đã seed.

## Cú pháp

```text
zenify wt new <slug> [flags]
```

## Khi nào dùng

Trước lần sửa code đầu tiên của một task. Mọi thay đổi code nằm trong worktree, checkout chính chỉ để đọc và chạy.

## Kết quả

Lệnh tạo worktree trong `.worktrees/<slug>`, checkout branch `<user>/<type>/<slug>` từ base ref đã khai trong `.claude/worktree.json`, cấp một port trong dải của repo, copy các file khai ở `copy` và cài deps theo `deps`. Lệnh in đường dẫn worktree và port.

Với `deps: clone`, lệnh copy `node_modules` từ nguồn clone rồi chạy lệnh `install`. Nguồn clone là slot cache khoá theo hash `package-lock.json` cộng dấu vân tay môi trường (Node ABI/platform/arch) khi có slot khớp lock của worktree và đúng môi trường hiện tại (xem đoạn cache bên dưới), không thì là checkout chính. Khi `install` đúng là `npm ci`, `npm install` hoặc `npm i`, lệnh bỏ qua bước cài nếu bản copy đã khớp: `package-lock.json` của worktree giống hệt bản đi kèm nguồn clone (slot cache hoặc checkout chính) và khớp dependency trong `package.json`, còn `node_modules` của nguồn clone khớp lock theo `node_modules/.package-lock.json` của npm (mọi gói đã cài có trên đĩa, đúng version, nguồn và integrity; gói chưa cài chỉ là gói optional của nền tảng khác). Lệnh in `skipping "npm ci"` rồi vẫn chạy các script `preinstall`, `postinstall`, `prepare` có trong `package.json` bằng `npm run` (kèm `preprepare`/`postprepare` như `npm ci`; ví dụ `husky` cài git hook cho worktree); khi npm đặt `ignore-scripts=true` (biến môi trường hoặc `.npmrc`) thì không chạy script nào, nên worktree tạo nhanh và bản copy copy-on-write gần như không tốn thêm disk. Lệnh vẫn cài đủ như cũ khi lock khác nhau, `node_modules` của nguồn clone lệch lock (chưa cài lại sau khi pull, cài `--omit=dev`, gói bị xoá), repo có `npm-shrinkwrap.json`, script `install`, `prepublish` hoặc hook mà `npm ci` không chạy (ví dụ `prepostinstall`), gói workspace (`link`), lệnh `install` tuỳ biến, hoặc không có `package-lock.json` (yarn, pnpm). Cần cài lại dù lock giống nhau thì dùng `--install`.

Với `deps: clone`, lệnh còn dùng một cache deps bật sẵn, chỉ ở chế độ clone (`symlink` luôn trỏ thẳng về checkout chính và không đọc hay ghi cache). Cache nằm ở `~/.cache/zenify/wt-deps` (hoặc `$XDG_CACHE_HOME/zenify/wt-deps` nếu biến này được đặt), mỗi repo một thư mục, mỗi slot khoá theo hash của `package-lock.json` cộng dấu vân tay môi trường build (Node ABI/platform/arch): `<cache>/<repo>/<hash>/{node_modules,package-lock.json}`. Khi có slot khớp lock của worktree và đúng môi trường hiện tại, lệnh copy `node_modules` từ slot đó thay vì từ checkout chính, nên `wt new` lặp lại vẫn nhanh dù checkout chính đang ở branch nào. Chỉ một cây `node_modules` thật sự do `npm ci` cài xong dưới Node hiện tại mới được ghi vào cache: lệnh kiểm tra nội dung cây là khớp lock (không chỉ dựa vào tên lệnh), và bỏ qua ghi cache nếu `node_modules` cài với `ignore-scripts=true` (deps chưa build) hoặc nếu không dò được môi trường Node; sau đó tự giữ tối đa 5 slot mỗi repo (xoá slot ít dùng gần đây nhất trước). Đặt `ZENIFY_DEPS_CACHE=<đường dẫn>` để đổi thư mục cache, hoặc `ZENIFY_DEPS_CACHE=off` (hay để rỗng) để tắt hẳn. Vì slot khoá theo Node ABI, đổi Node (ví dụ qua nvm) tự cho cache miss và dùng slot khác — không cần xoá tay. `ZENIFY_DEPS_CACHE=off` và `--install` chỉ bỏ qua việc đọc cache, không xoá slot cũ.

Sau khi tạo xong, `wt new` đẩy branch lên `origin` (`push -u`) để việc hiện là đã bắt đầu. Lệnh bỏ qua bước này với `--type hotfix` hoặc khi repo không có `origin`. Fetch `origin` lỗi (offline) thì lệnh không push mà in nhắc tự push khi có mạng. Push lỗi chỉ in cảnh báo (một dòng, thông tin đăng nhập trong URL bị che), worktree vẫn được tạo. Nếu `origin` đã có branch cùng tên với commit khác, cảnh báo nói rõ và lệnh không ghi đè. Branch đã có sẵn trên `origin` không được tính là do `wt new` tạo, nên `wt rm` sau này không bao giờ xóa nó. Khi `wt new` tự dọn một worktree đã xong, nó giữ nguyên branch remote của worktree đó.

`<user>` lấy theo thứ tự: trường `user` trong `worktree.json` nếu repo có khai, rồi `git config user.name` (`Nguyen An` thành `Nguyen-An`), rồi phần trước `@` của `git config user.email` (`an.nguyen@example.com` thành `an.nguyen`), rồi biến môi trường `$USER` (Windows: `$USERNAME`). Ký tự git không cho phép trong tên branch được thay bằng `-`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--type` | Loại task: `feat`, `fix`, `chore` hoặc `hotfix`. Mặc định `feat`. `hotfix` đổi base ref sang `hotfixBaseRef`. |
| `--base` | Base ref tự chọn, thắng mọi giá trị trong `worktree.json`. |
| `--another` | Cho phép mở task thứ hai trong repo khi đã có một worktree chưa merge. |
| `--install` | Buộc cài deps mới dù config khai `symlink` hoặc `clone`. |

Slug mang mã ticket khi spec hoặc `## Intent` có dòng `**Việc Plane:**`, ví dụ `zen-123-session-timeout` cho branch `<user>/fix/zen-123-session-timeout`. Mã `zen-<n>` trong tên branch là thứ `/znf:ship` đọc để đặt `ZEN-<n>` vào tiêu đề PR.

## Ví dụ

```bash
git fetch origin
zenify wt new session-timeout --type fix
```

## Lưu ý

Lệnh từ chối khi repo đã có worktree chưa merge của bạn và in lệnh `cd` vào worktree đó. Dùng `--another` chỉ khi đó thực sự là việc khác. Xem thêm [Mỗi slug một worktree](/concepts/worktree-per-slug).

## Lệnh liên quan

- [zenify wt](./zenify_wt): Nhóm lệnh quản lý worktree theo slug: tạo, liệt kê, gỡ, dọn, kèm port và môi trường dev riêng cho từng task.

<!-- Generated by zenify docs gen: prose from internal/docsgen/catalog, facts from the command tree. -->
