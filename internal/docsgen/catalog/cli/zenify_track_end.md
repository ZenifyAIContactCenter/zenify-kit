---
summary: Đóng một lượt theo dõi với PR, branch, rollback hoặc cancelled; đếm số task từ ledger SDD.
---
## Khi nào dùng

Sau khi `znf:ship` mở PR, hoặc ở step 10 của hotfix. Id chưa start hoặc đã end thì lệnh báo lỗi và không ghi.

## Ví dụ

```bash
zenify track end fix-261030-ab12 --pr https://github.com/org/repo/pull/1 --ledger progress.md --rounds 1
```
