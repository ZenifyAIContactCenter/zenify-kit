---
summary: Liệt kê key trong `.zenify/.env`: có hay trống, nguồn, mô tả; chỉ in giá trị của key không phải secret.
---
## Kết quả

Mỗi key một dòng: tên, `set` hoặc `empty`, nguồn `manifest` hoặc `local`, giá trị (chỉ khi key có `secret: false`), mô tả. Key có trong file mà không có trong manifest là `local` và coi như secret, nên giá trị không bao giờ được in.

## Ví dụ

```bash
zenify env list
```
