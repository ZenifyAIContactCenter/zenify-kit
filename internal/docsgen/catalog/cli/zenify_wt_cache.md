---
summary: Nhóm lệnh ghi và đọc cache deps `node_modules`, dùng chung slot với `wt new`.
---
## Khi nào dùng

Khi một môi trường không chạy `wt new` nhưng vẫn cần cache deps mà `wt new` dùng, ví dụ sandbox worker cài `node_modules` một lần rồi tái dùng cho lần sau. `put` ghi cache, `get` đọc lại.

Cả hai dùng đúng cache và cách đặt slot của `wt new` (`<cache>/<repo>/<hash>/{node_modules,package-lock.json}`, khoá theo hash `package-lock.json` cộng dấu vân tay Node ABI/platform/arch), nên slot do lệnh nào ghi cũng được lệnh kia dùng. Xem đoạn cache trong [`zenify wt new`](/reference/cli/zenify_wt_new).
