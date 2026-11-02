---
summary: Ghi một vòng fix của lượt fix/hotfix: đường chẩn đoán narrow hoặc wide và có regression test hay không.
---
## Khi nào dùng

Ở bước Report của `znf:fix` và step 10 của `znf:hotfix` trên đường đã sửa code.

## Ví dụ

```bash
zenify track fix fix-261030-ab12 --round 1 --path narrow --regression test
```
