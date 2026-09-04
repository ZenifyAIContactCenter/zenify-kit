---
name: analyze
description: Use to inspect a written spec+plan pair BEFORE implementing — mechanically checks requirement coverage (FR→task), leftover clarification markers, and Brief structure, then adds judgment on SC-testability, necessity, and DB-safety. advisory only, never blocks.
allowed-tools: Read Grep Bash(zenify analyze *)
---

# znf:analyze — soi spec+plan trước khi code

**Announce:** "Using znf:analyze to inspect the spec+plan before implementation."

Kiểm một cặp spec+plan đã viết, đối chiếu với `znf:_shared/constitution` (P1–P8) và
`znf:_shared/spec-template` (Brief 7-trường). **Advisory:** báo findings, KHÔNG chặn tiến độ.
Hai lớp — cơ học (command) rồi phán đoán (skill).

## Khi nào dùng

- Sau khi có spec **và** plan, trước khi dispatch implementer (cook gọi ở đây).
- Hoặc gọi tay: `/analyze <spec.md> <plan.md>` trên bất kỳ cặp nào.

## Bước 1 — quét cơ học (tất định)

Chạy command đọc coverage/marker/structural — phần này KHÔNG để LLM đếm tay:

```
zenify analyze --spec <spec-path> --plan <plan-path>
```

Đọc output:
- **Coverage** — orphan FR (yêu cầu không task nào làm, CRITICAL), orphan task (task không khai
  `_Requirements:`, HIGH), dangling ref (plan cite FR spec không có, HIGH).
- **Marker** — mọi `[NEEDS CLARIFICATION` còn sót (HIGH), kèm số dòng.
- **Brief** — có `## Brief` không, mấy/7 mục.

Command fail-open: nếu nó báo "không phân tích được", ghi nhận và tiếp — đừng coi là lỗi chặn.

## Bước 2 — ba pass phán đoán (thứ command không làm được)

Đọc spec+plan bằng mắt và phán đoán, mỗi phát hiện severity **MEDIUM**:

1. **SC-testable (constitution P3).** Mỗi SC có dạng Given/When/Then hoặc một câu "the system
   shall" kiểm được không? SC prose-only, không kiểm được → MEDIUM.
2. **Necessity (P6).** Trường "cách làm" của Brief có justify *đường có sẵn nào chưa lo được việc
   này* không (necessity ladder)? Thiếu justify khi có xây mới → MEDIUM.
3. **db-3 + comprehension floor (P7).** Khối DB-guarantees có thật (nêu query-plan / tenant-scope /
   keyset, hoặc N/A có lý do) hay hand-wave? Spec có mô tả luồng thật + blast-radius trước khi đề
   xuất cắt gì không? Thiếu → MEDIUM.

## Bước 3 — báo cáo

Gộp findings cơ học + phán đoán, sắp theo severity (CRITICAL → HIGH → MEDIUM), in gọn.
**Mở đầu báo cáo bằng: "Advisory — không chặn tiến độ."** Skill này không có quyền dừng luồng;
nêu findings để người dùng (hoặc cook) quyết sửa spec/plan hay tiếp.
