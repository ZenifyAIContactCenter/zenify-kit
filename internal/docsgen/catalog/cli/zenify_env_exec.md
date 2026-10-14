---
summary: Chạy một lệnh với key trong `.zenify/.env`, che giá trị secret trong stdout và stderr.
---
## Khi nào dùng

Chạy script tay cần credential mà không muốn export vào shell. Môi trường lệnh con là env hiện có cộng key trong file; trùng tên thì env hiện có thắng, trừ khi nó rỗng.

## Kết quả

Stdout và stderr được che theo dòng: mọi giá trị secret dài từ 6 ký tự trở lên bị thay bằng `***`. Exit code của lệnh con trở thành exit code của `zenify`.

## Ví dụ

```bash
zenify env exec -- node scripts/check.js
```
