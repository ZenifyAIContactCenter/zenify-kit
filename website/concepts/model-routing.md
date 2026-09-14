---
title: Chọn model
---

# Chọn model: session, skill, subagent

Model được đặt ở **hai chỗ**, và đó là thứ quyết định cách đổi nó:

- **Session model** — vòng lặp chính, đặt bằng `/model`, ghim full ID trong `~/.claude/settings.json`. **Mọi skill chạy ở đây** và luôn dùng model này.
- **Subagent model** — agent dispatch qua Agent tool: ghim riêng (frontmatter full ID, hoặc tham số `model` enum), hoặc kế thừa session khi không đặt.

Một skill **không tự nâng model được**. Muốn một bước bắt buộc chạy Opus: giữ session ở Opus, hoặc đẩy việc đó qua agent có frontmatter ghi full ID.

## Việc nào chạy model nào

Model mạnh điều phối, model rẻ làm phần song song — một agent tốn ~4× token so với chat, multi-agent ~15×, và Opus dẫn dắt + Sonnet subagent vượt Opus đơn lẻ ([multi-agent](https://www.anthropic.com/engineering/multi-agent-research-system)).

| Nơi | Model | Vì sao |
|---|---|---|
| Agent `code-reviewer` | `claude-opus-4-8` | Review nặng phán đoán; `ship` hạ tier cho diff nhỏ |
| Agent `scout`, `ui-verifier` | `sonnet` | Dò/đọc-tra, lái browser; không cần suy luận sâu |
| `cook` bước 0–5 (brainstorm→analyze) | Session (Opus) | Quyết định thiết kế, chạy ở vòng lặp chính |
| `cook` bước 6 implementer | `sonnet` + effort `xhigh` | Sàn sonnet, không haiku; task phán đoán thì bỏ `model` lên Opus |
| `review` T1 | `sonnet` <50 LOC / mid | Diff nhỏ dùng model rẻ |
| `review` T2 fan-out | `sonnet`; `contracts` lên `opus` khi chạm contract chia sẻ | Sâu hơn ở phần rủi ro |
| `review` T3 | `security`+`contracts` `opus`; `bugs`/`perf`/`types` `sonnet`; verify `opus` | Blast radius auth/contract cần model mạnh |
| `review` adviser · `fix` · `gate` | `sonnet` | Chỉ đọc / điều tra / quét cross-repo |
| `Explore` | kế thừa session | Tìm kiếm rộng |

## Đổi model

- **Xuống tier:** đặt `model: 'sonnet'`. **Lên top tier:** **bỏ** tham số `model` (kế thừa session) — đừng đặt alias `opus`, nó trôi sang bản mới nhất.
- **Effort quan trọng hơn tier** với code: `claude-haiku-4-5` không hỗ trợ `xhigh` (bị âm thầm hạ effort, không báo lỗi), nên sàn implementer là sonnet.

## Ghi chú

- **Ghim full ID, không dùng alias.** `opus` trỏ bản opus mới nhất và âm thầm override phiên bản đã ghim — với team, đó là cả team đổi hành vi một đêm mà không ai review. Full ID khóa hành vi lại; nâng cấp là bump con số sau khi kiểm chứng qua gate + luồng thật.
- **Chưa lên Opus 5.** Không phải vì nó kém — Anthropic công bố ngang giá 4.8 và dẫn đầu benchmark hard-agentic ([opus-5](https://www.anthropic.com/news/claude-opus-5)). Nhưng chính [hướng dẫn prompt Opus 5](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/prompting-claude-opus-5) của Anthropic nêu các mặc định gây tranh cãi: dài hơn, tự verify khi không được yêu cầu, mở rộng scope ngoài yêu cầu, delegate nhiều hơn. Ghim giữ cho team quyền đổi có kiểm chứng thay vì để alias tự quyết.
- **"Bỏ harness khi model mạnh" chỉ đúng với prompt thừa.** Anthropic tắt mặc định `TodoWrite`/`Task` trên model mới vì chúng tự track — đó là bỏ prompt, không phải bỏ kiến trúc. Gate xác định, route model rẻ, tái lập cả team, quan sát được: không phần nào là prompt scaffolding, nên giữ.

## Liên quan

- [Gate fail-open](/concepts/gate-fail-open) — các gate xác định là một nửa harness.
- [Ba lớp](/concepts/three-layers) — nơi các lựa chọn model sống.
- [Review và gate](/guides/review-and-gates) — tier review chọn model theo kích thước và blast radius.

<!-- Nguồn (cho người bảo trì, không hiển thị):
Nội bộ: settings.json "model": claude-opus-4-8; znf/skills/onboard-project/SKILL.md §Model routing rule; znf/agents/{code-reviewer=opus-4-8, scout/ui-verifier=sonnet}; znf/skills/cook/SKILL.md + references/step6-implementation-notes.md (haiku not xhigh-capable, silent downgrade); znf/skills/review/SKILL.md + workflows/review-changes.js; fix/gate SKILL.md; memory todowrite-gated-by-model-version.md
Web (fetched 2026-09-14, nguồn chính = Anthropic; tin cộng đồng chỉ làm màu):
- anthropic.com/news/claude-opus-5 (giá = 4.8, SOTA hard-agentic, "verifies its work and iterates")
- platform.claude.com/docs/.../prompting-claude-opus-5 (Anthropic tự nêu verbosity, over-verify, scope-expansion, over-delegation)
- anthropic.com/engineering/multi-agent-research-system (4×/15×; Opus-lead + Sonnet-subagents > single Opus; effort ladder 1/2-4/10+)
- claude.com/docs/.../todo-tracking (TodoWrite/Task off mặc định trên model mới)
- Reception thin/sentiment: HN 49079191 (chia hai phía); revert path /model claude-opus-4-8. KHÔNG verify: revert "4.7", tỉ lệ định lượng, Fable/Mythos model card, con số 80% cut.
-->
