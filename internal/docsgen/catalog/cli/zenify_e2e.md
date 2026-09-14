---
summary: Nhóm lệnh cho journey E2E: lint chống test hời và chạy journey Playwright thật trong Docker.
---
## Tổng quan

Journey E2E nằm trong thư mục `.znf/e2e` của repo. Bạn dùng `lint` để chặn journey thiếu re-fetch, assert hoặc cleanup, và `run` để chạy journey trên dev server đang mở. Cách viết journey xem [Kiểm thử UI](/workflows/ui-testing).
