---
summary: Chẩn đoán trước, sửa sau. Dùng khi có thứ gì đó đang lỗi mà bạn chưa rõ nguyên nhân.
---
## Khi nào dùng

Khi có lỗi và bạn chưa biết nguyên nhân thật sự. Xem [fix: sửa lỗi chưa rõ nguyên nhân](/workflows/fix). Nếu lỗi đang xảy ra trên production, cân nhắc `/znf:hotfix` thay vì skill này.

## Cách hoạt động

1. Skill lấy dữ liệu lỗi thật trước khi suy đoán: log gần nhất, output test đang fail, hoặc log CI nếu bạn đưa link GitHub Actions.
2. Skill xác nhận nguyên nhân gốc trước khi viết bất kỳ dòng sửa nào. Với lỗi rõ ràng, nó kiểm một giả thuyết; với lỗi khó, nó chạy song song nhiều giả thuyết rồi loại dần bằng bằng chứng thật.
3. Worktree được tạo trước bước sửa, không trước bước chẩn đoán, dựa trên base ref repo khai báo. Nếu đang có worktree của task khác cho cùng repo trong phiên này, skill dùng lại worktree đó thay vì tạo mới.
4. Skill gọi `/znf:scout` để tìm ai phụ thuộc vào đoạn code sắp sửa, trước khi sửa. Nếu việc sửa hoá ra cần hơn khoảng ba file hoặc đổi một hợp đồng dịch vụ chung, skill dừng lại và đề nghị chuyển sang `/znf:cook`.
5. Sau khi sửa, skill xác minh lỗi đã hết bằng cách chạy lại đường code thật, rồi gọi `/znf:gate` và luôn gọi `/znf:ship` sau đó, kể cả khi gate báo không có tác động liên repo.

## Ví dụ

```text
/znf:fix "Trang danh sách ticket bị treo khi lọc theo tag"
/znf:fix https://github.com/org/repo/actions/runs/123456
/znf:fix
```

## Lưu ý

Không đối số nghĩa là skill tự tìm log lỗi gần nhất. Nếu không xác nhận được nguyên nhân gốc, skill nói rõ điều đó thay vì đoán bừa rồi sửa.
