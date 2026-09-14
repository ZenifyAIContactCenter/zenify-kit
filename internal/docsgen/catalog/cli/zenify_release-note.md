---
summary: Ghi một commit ghi chú release mang metadata rủi ro, để release report đọc sau.
---
## Khi nào dùng

Ở bước cuối của `/znf:ship`, trước khi push branch. Ship chạy lệnh này tự động với giá trị lấy từ ba tag rủi ro trong Brief của spec. Bạn chạy tay khi ship không có spec để lấy giá trị.

## Kết quả

Lệnh tạo một commit rỗng có tiêu đề `chore(release): note <slug>` và phần thân gồm các trailer: slug, mô tả, blast radius, DB, cách rollback, và đường dẫn spec nếu có. `zenify release-report` gom các commit này thành báo cáo rủi ro của release.

Thiếu `--slug` thì lệnh bỏ qua và không chặn. Commit lỗi cũng không chặn ship.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--slug` | Slug của thay đổi. Bắt buộc. |
| `--note` | Mô tả một dòng. |
| `--blast` | Phạm vi ảnh hưởng. Mặc định `unknown`. |
| `--db` | Thay đổi DB. Mặc định `N/A`. |
| `--rollback` | Cách rollback. Mặc định `revert PR`. |
| `--spec` | Đường dẫn spec, tùy chọn. |
| `--dir` | Thư mục repo. Mặc định thư mục hiện tại. |

## Ví dụ

```bash
zenify release-note --slug kit-docs-site --note "Site tài liệu VitePress" \
  --blast "zenify-kit" --db "N/A" --rollback "revert PR"
```
