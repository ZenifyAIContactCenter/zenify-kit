---
summary: Nhóm lệnh ghi/kiểm tra bằng chứng UI-verify cho gate cơ học của /znf:ship.
---
## Tổng quan

Gồm `record` (agent `ui-verifier` ghi lại screenshot + số đo layout keyed theo fingerprint hiện tại) và `check` (`/znf:ship` bước 7 dùng để gate fail-closed trước khi kết luận Shippable). Xem [Kiểm thử UI](/workflows/ui-testing).
