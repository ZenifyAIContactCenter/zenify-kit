---
title: "Namespace znf:"
---

# Namespace `znf:`

## Tổng quan

Nhiều người trong team đã có skill cá nhân tên `/cook`, `/fix`, `/ship` từ trước khi cài kit. Nếu plugin của kit đăng ký cùng tên, bản nào được nạp sẽ tùy thứ tự load và khó đoán.

Namespace `znf:` tránh việc này bằng cách không dùng chung tên. Mọi skill của kit gọi qua `/znf:<name>`, ví dụ `znf:cook`, `znf:fix`, `znf:ship`. Bản cá nhân và bản chuẩn team cùng tồn tại, không tranh chấp.

## Cách hoạt động

| Nhóm | Skill (gọi `/znf:<name>`) | Vai trò |
|---|---|---|
| Vòng đời tính năng | `cook`, `ground`, `scout`, `ship` | brainstorm, spec, ground, plan, implement, gate |
| Sự cố | `fix`, `hotfix` | debug với log thật; sửa production từ base release |
| Hợp đồng chia sẻ | `gate`, `contract-sweep`, `explain-plan` | sweep cross-repo, DB-perf gate |
| Vận hành | `run`, `sweep`, `prune-memory`, `retro`, `where` | chạy app thật, dọn worktree, dọn memory, rút bài học của phiên, tóm tắt task tới đâu |
| Domain skill | `mongo-data-safety`, `sql-data-safety`, `mongoose-modeling`, `service-integration`, `express-service-patterns`, `nestjs-patterns`, `react-patterns` | idiom theo stack; không gọi tay mà được route: bảng `_shared/skill-routing` ánh xạ tín hiệu trong task (chạm Mongo, SQL, schema, pub/sub, Express, NestJS, React) sang tên skill, plan ghi kết quả vào tag `_Skills:` của từng task, implementer invoke trước lần sửa đầu. Skill `<repo>-conventions` vẫn nằm trong từng repo |

Danh sách đầy đủ sinh tự động từ binary, xem [tham chiếu Skill](/reference/skills/).

Trong workspace zenify, khi một prompt gọi một verb có cả bản `znf:` và bản cá nhân, bạn ưu tiên bản `znf:`. Đó là chuẩn team. Bản cá nhân là mặc định cho project khác chưa cài kit.

Ba agent mà các skill này dispatch là `code-reviewer`, `scout`, `ui-verifier`. Chúng nhúng trong cùng plugin nhưng không mang tiền tố `znf:`. Agent không có namespace riêng như skill và được gọi trực tiếp bằng tên, ví dụ `Agent(subagent_type: "scout")`.

## Liên quan

- [Ba lớp: binary, plugin, knowledge store](/concepts/three-layers): plugin skill là lớp chứa các skill `znf:*`.
- [Chọn workflow](/workflows/): chọn cook, fix, hotfix hay không dùng workflow theo mức hiểu về công việc, không theo tên skill.
- Tham chiếu: [Skill](/reference/skills/), [Agent](/reference/agents/).

## Lưu ý

Một số skill khai báo `disable-model-invocation: true` trong frontmatter. Ví dụ `contract-sweep`, vì nó fan-out một agent cho mỗi repo và tốn token thật. Skill loại này không được Claude tự gọi dựa trên ngữ cảnh. Nó chỉ chạy khi bạn gõ `/znf:contract-sweep` bằng tay.

Khi viết skill mới có fan-out tốn kém, hãy nhớ cờ này. Thiếu cờ, mô hình có thể tự dispatch fan-out mà không ai yêu cầu.

Description trong frontmatter của mọi skill và agent tối đa 300 ký tự; `go test` fail khi vượt (`internal/docsgen/description_test.go`). Description luôn nằm trong context để model chọn skill, nên viết nó như điều kiện kích hoạt (khi nào dùng), không tóm tắt các bước. Rút gọn mà làm rơi một nhánh kích hoạt thì model sẽ bỏ lỡ skill đó với yêu cầu tương ứng.

<!-- Nguồn (cho người bảo trì, không hiển thị):
- `internal/plugin/assets/znf/skills/gate/SKILL.md` (ví dụ `disable-model-invocation: true` trên `contract-sweep`)
- `website/reference/skills/index.md`, `website/reference/agents/index.md` (danh sách sinh tự động, agent không mang tiền tố `znf:`)
-->
