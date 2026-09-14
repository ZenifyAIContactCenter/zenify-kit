---
title: "Namespace znf:"
---

# Namespace `znf:`

## Vấn đề nó giải quyết

Nhiều người trong team đã có sẵn skill cá nhân tên `/cook`, `/fix`, `/ship`, … từ trước khi cài kit. Nếu plugin của kit cũng đăng ký đúng những tên đó, một trong hai bản sẽ thắng một cách khó đoán, tuỳ thứ tự nạp. Namespace `znf:` giải quyết việc này bằng cách không dùng chung tên: mọi skill của kit gọi qua `/znf:<name>` — `znf:cook`, `znf:fix`, `znf:ship`, … — nên bản cá nhân và bản chuẩn team cùng tồn tại, không tranh chấp.

## Mô hình tư duy

| Nhóm | Skill (gọi `/znf:<name>`) | Vai trò |
|---|---|---|
| Vòng đời tính năng | `cook`, `ground`, `scout`, `ship` | brainstorm → spec → ground → plan → implement → gate |
| Sự cố | `fix`, `hotfix` | debug có log thật; vá production từ base release |
| Hợp đồng chia sẻ | `gate`, `contract-sweep`, `explain-plan` | sweep cross-repo, DB-perf gate |
| Vận hành | `run`, `sweep`, `prune-memory` | chạy app thật, dọn worktree, dọn memory |

Danh sách đầy đủ, sinh tự động từ chính binary, nằm ở [tham chiếu Skill](/reference/skills/). Trong workspace zenify, quy ước là: khi một prompt gọi đúng một verb workflow đã có cả bản `znf:` lẫn bản cá nhân, ưu tiên bản `znf:` — đó là chuẩn team, còn bản cá nhân là default chung cho project khác chưa cài kit.

Hai agent mà các skill này dispatch — `code-reviewer` và `scout` — cũng nhúng trong cùng plugin, nhưng **không** mang tiền tố `znf:`: agent không có namespace riêng như skill, chúng được gọi trực tiếp bằng tên (`Agent(subagent_type: "scout")`).

## Ghép với …

- [Ba lớp: binary, plugin, knowledge store](/concepts/three-layers) — plugin skill là lớp vật lý chứa các skill `znf:*` này. <!-- TODO(Task 9): link /workflows/ để nói rõ cook/fix/hotfix chọn thế nào -->
- Tham chiếu: [Skill](/reference/skills/), [Agent](/reference/agents/).

## Edge case

Một số skill khai báo `disable-model-invocation: true` trong frontmatter — ví dụ `contract-sweep`, vì nó fan-out một agent mỗi repo nên tốn token thật. Skill loại này **không** được Claude tự quyết định gọi dựa trên ngữ cảnh; nó chỉ chạy khi bạn gõ đúng `/znf:contract-sweep` bằng tay. Quên cờ này khi viết skill mới đồng nghĩa với việc để mô hình tự ý dispatch một fan-out tốn kém mà không ai yêu cầu.

## Nguồn

- `docs/handoff/zenify-kit/m2-plugin-skills.md` (`zenify skills sync` materialize `znf/` vào `~/.claude/skills/znf`, prefix `znf:` để cùng tồn tại với skill cá nhân)
- `internal/plugin/assets/znf/skills/gate/SKILL.md` (ví dụ `disable-model-invocation: true` trên `contract-sweep`)
- `website/reference/skills/index.md`, `website/reference/agents/index.md` (danh sách sinh tự động, agent không mang tiền tố `znf:`)
- Ground trên binary build từ commit bf91c62 của nhánh này (2026-09-14), chưa phát hành.
