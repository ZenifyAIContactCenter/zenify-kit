---
title: Chọn model
---

# Chọn model: session, skill, subagent

Kit theo hai nguyên tắc, cả hai đi ngược trực giác "cứ chọn bản mạnh nhất, mới nhất":

1. **Ghim một phiên bản đã kiểm chứng cho cả team.** Nâng cấp là quyết định có review, không phải trôi theo bản mới.
2. **Mỗi việc chạy trên model rẻ nhất làm được nó.** Top tier để dành cho suy luận khó.

## Model được chọn ở hai chỗ

- **Session model** — vòng lặp chính, đặt bằng `/model`, ghim trong `~/.claude/settings.json`. **Mọi skill chạy ở đây.**
- **Subagent model** — model của agent dispatch qua Agent tool.

Một skill **không tự nâng model được**: nó luôn dùng session model. Chỉ subagent mới ghim model riêng (frontmatter full ID, hoặc tham số `model` enum), hoặc kế thừa session khi không đặt. Vậy một bước bắt buộc Opus: giữ session ở Opus, hoặc đẩy qua agent có frontmatter ghi full ID.

## Nguyên tắc 1: ghim đúng phiên bản

Session ghim full ID `claude-opus-4-8`, **không** dùng alias `opus`:

- Alias `opus` trỏ bản opus **mới nhất** và âm thầm override phiên bản đã ghim. Với team, đó là **cả team đổi hành vi một đêm** mà không ai review — gate, prompt, ngưỡng có thể lệch trong im lặng.
- Full ID khóa hành vi lại: cả team tái lập được, và nâng cấp là bump con số sau khi kiểm chứng qua gate + luồng thật.

Cùng lý do, **đừng dùng alias `opus`** ở agent frontmatter hay lệnh dispatch. Để chạm top tier từ skill thì **bỏ** tham số `model` (kế thừa session), đừng đặt tên nó.

**Vì sao chưa lên Opus 5.** Không phải vì Opus 5 kém — Anthropic công bố nó ngang giá 4.8 và dẫn đầu benchmark hard-agentic ([opus-5](https://www.anthropic.com/news/claude-opus-5)). Nhưng chính [hướng dẫn prompt Opus 5](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/prompting-claude-opus-5) của Anthropic tài liệu hóa các hành vi mặc định gây tranh cãi: trả lời dài hơn, tự verify khi không được yêu cầu, **mở rộng scope task ngoài yêu cầu**, delegate subagent nhiều hơn (tốn chi phí ở task nhỏ). Một bộ phận dev vì thế quay về 4.8 (đường lùi chính thức `/model claude-opus-4-8`) — cảm nhận vocal, không phải regression đo được. Ghim giữ cho team quyền thử và đổi có kiểm chứng, thay vì bị alias tự quyết.

## Nguyên tắc 2: mỗi việc trên model rẻ nhất

Có dẫn chứng số từ Anthropic ([multi-agent](https://www.anthropic.com/engineering/multi-agent-research-system)): một agent tốn ~4× token so với chat, multi-agent ~15×; và **Opus dẫn dắt + Sonnet subagent vượt Opus đơn lẻ**. Model mạnh điều phối, model rẻ làm phần song song.

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

**Scale bất đối xứng:** xuống thì ghi `model: 'sonnet'`; lên top tier thì **bỏ** `model`. Và với code, **effort quan trọng hơn tier** — `claude-haiku-4-5` không hỗ trợ `xhigh` (bị âm thầm hạ effort, không báo lỗi), nên sàn implementer là sonnet.

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
- Reception thin/sentiment: HN 49079191 (chia hai phía); revert path /model claude-opus-4-8. KHÔNG verify: revert "4.7", tỉ lệ định lượng, Fable/Mythos model card, con số 80% cut.
-->
