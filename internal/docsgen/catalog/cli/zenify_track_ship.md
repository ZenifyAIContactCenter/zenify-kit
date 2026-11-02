---
summary: Ghi kết quả một lần chạy znf:ship: shipped hay blocked, bước bị chặn, số finding và số vòng fix.
---
## Khi nào dùng

Cuối mỗi lần chạy `znf:ship` khi có id đang mở. `--blocked-step` bắt buộc khi blocked.

## Ví dụ

```bash
zenify track ship fix-261030-ab12 --outcome blocked --blocked-step lint --findings 0,1,2,0 --rounds 1
```
