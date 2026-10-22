---
summary: Chọn đúng package manager JS của project (pnpm, yarn, npm, bun) rồi chạy nó, để không sinh thêm lockfile thứ hai.
---
## Khi nào dùng

Khi cần chạy `install`, `run <script>` hay `test` trong một repo JS mà bạn không chắc nó dùng manager nào. Skill của kit gọi `pm` thay vì đoán; `zenify up` sẽ cài shim `~/.local/bin/pm` trỏ về lệnh này.

Thứ tự quyết định: trường `packageManager` trong `package.json` gần nhất (phần trước dấu `@`), rồi lockfile có mặt theo thứ tự pnpm, yarn, npm, bun. Không có tín hiệu nào thì lệnh từ chối và thoát mã 1, không tự chọn. Chạy được từ thư mục con vì lệnh tìm `package.json` ở các thư mục cha.

## Kết quả

Không có tham số: in tên manager ra stdout, còn `root:` và `reason:` ra stderr. Có tham số: chạy manager đó tại thư mục chứa `package.json`, truyền nguyên tham số, stdin/stdout/stderr và mã thoát.

## Ví dụ

```bash
zenify pm
zenify pm install
zenify pm run dev
```
