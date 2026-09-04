---
name: standards
description: Use after implementing a plan — checks test-traceability: every FR/SC has a real test on disk, not just a testable-shaped SC. Mechanical command + judgment on whether the test truly asserts the requirement. advisory only, never blocks.
allowed-tools: Read Bash(zenify standards *)
---

# znf:standards — mỗi requirement có test thật

**Announce:** "Using znf:standards to check test-traceability."

M5b kiểm một SC **có dạng** testable; skill này kiểm requirement **có test thật** trong code —
chạy **sau khi implement**. Đối chiếu FR/SC ↔ test file khai trong plan, kiểm trên đĩa.
**Advisory:** báo findings, KHÔNG chặn tiến độ. Hai lớp — cơ học (command) rồi phán đoán (skill).

## Khi nào dùng

- Sau khi SDD implement xong plan, trước/tại ship (cook gọi ở Step 6b).
- Hoặc gọi tay: `/standards <spec.md> <plan.md>` trên một cặp đã implement.

## Bước 1 — quét cơ học (tất định)

```
zenify standards --spec <spec-path> --plan <plan-path> --root <repo-root>
```

Command tái dùng coverage FR→task của `znf:analyze`, thêm kiểm test file trên đĩa:
- `untested-fr` (HIGH) — FR có task phủ nhưng task đó không khai test nào.
- `missing-test-file` (HIGH) — path test khai trong plan không có trên đĩa.
- `empty-test-file` (MEDIUM) — file test tồn tại nhưng không có test func (language-aware).
- `unchecked-lang` (INFO) — đuôi lạ, chỉ kiểm tồn tại, không kiểm nội dung.

Fail-open: command luôn exit 0, không bao giờ chặn.

## Bước 2 — phán đoán (2 pass, MEDIUM, skill làm — command không làm được)

- **Pass A — test có THẬT SỰ assert requirement không.** Đọc (`Read`) vài test file KHÔNG bị flag:
  một `func TestX` rỗng hay chỉ `assert True`/`expect(true)` vẫn qua Bước 1 nhưng không kiểm gì.
  Báo test nào tồn tại mà assertion trống/trivial so với FR nó gắn.
- **Pass B — mỗi SC Given/When/Then có một assertion tương ứng không.** Đối chiếu SC trong spec với
  assertion trong test: một nhánh When/Then không có test tương ứng là lỗ, dù FR tổng thể "có test".

## Bước 3 — báo cáo advisory

Mở đầu: "Advisory — không chặn tiến độ." Liệt kê findings cơ học + phán đoán, mỗi cái một dòng
(kind · FR/path · vì sao). Không block; người quyết chấp nhận hoặc sửa.
