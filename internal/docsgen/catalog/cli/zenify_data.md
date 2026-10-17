---
summary: Nhóm lệnh chạy script dữ liệu với credential DB do broker cấp, không để lộ giá trị trong output.
---
## Tổng quan

Script dữ liệu cần kết nối Mongo, MySQL, Redis hoặc lumi-agent. Lệnh `run` xin credential từ broker, chạy script của bạn với credential đó và che giá trị trong output. Credential không được ghi ra đĩa.
