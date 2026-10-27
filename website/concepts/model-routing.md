---
title: Chọn model
---

# Chọn model: session, skill, subagent

Model được đặt ở **hai chỗ**, và đó là thứ quyết định cách đổi nó:

- **Session model** — vòng lặp chính, đặt bằng `/model`, ghim full ID trong `~/.claude/settings.json`. **Mọi skill chạy ở đây** và luôn dùng model này.
- **Subagent model** — agent dispatch qua Agent tool: ghim riêng (frontmatter, hoặc tham số `model`), hoặc kế thừa session khi không đặt. Từ v0.32.0 mọi quy tắc chọn là **chữ viết sẵn trong skill**, không còn script tính model theo đặc điểm task.

Một skill **không tự nâng model được**. Muốn có ý kiến thứ hai từ model mạnh hơn, dùng [advisor native](#advisor-native) của Claude Code (mỗi người tự bật) hoặc đổi `/model` cho phiên.

## Việc nào chạy model nào

Model mạnh điều phối, model rẻ làm phần song song — một agent tốn ~4× token so với chat, multi-agent ~15×, và Opus dẫn dắt + Sonnet subagent vượt Opus đơn lẻ ([multi-agent](https://www.anthropic.com/engineering/multi-agent-research-system)).

**Nguyên tắc (theo superpowers): dùng model *ít mạnh nhất mà vẫn kham được role*.** Haiku hợp chỗ **miss thì rẻ**, nhưng KHÔNG hợp chỗ miss là false-negative *âm thầm và đắt* (review, chẩn đoán bug, drift cross-repo) — chỗ đó sàn sonnet.

| Nơi | Model | Vì sao |
|---|---|---|
| Vòng lặp chính (`cook` bước 0–5: brainstorm → analyze) | Session (Opus) | Quyết định thiết kế, chạy ở vòng lặp chính |
| `cook` bước 6 implementer (SDD) | Giá trị dòng `**Model:**` của task trong plan (`sonnet` hoặc `opus`); không có dòng thì `sonnet`. Cùng một test fail lần 2 → `opus` kèm brief lỗi trên đĩa; lần 3 → dừng implement, chạy investigator của `/fix` ở vòng 3 | Plan ghi quyết định, implementer luôn cần phán đoán; task cần phán đoán thiết kế ghi `opus` |
| `fix` investigator | Vòng 1 `sonnet`; vòng 2 `opus`; vòng 3 trở lên `opus` ở effort `xhigh`, kèm log, giả thuyết đã loại và diff đã thử | Chẩn đoán đầu sai thì lên tier, context mới |
| `review` T1 | `sonnet` khi diff dưới 50 LOC, `mid` ở các diff còn lại | Diff nhỏ dùng model rẻ; không xuống haiku — review haiku *grade tệ hơn* |
| `review` T2 fan-out | `sonnet`; `contracts` lên `opus` khi chạm contract chia sẻ | Sâu hơn ở phần rủi ro |
| `review` T3 | cả năm trục (`bugs`/`security`/`perf`/`contracts`/`intent`) `opus` | Blast radius auth/tenant/migration cần model mạnh; VERIFY cơ học, không còn vòng skeptic |
| Agent `code-reviewer`, `scout`, `ui-verifier`, `researcher` | `sonnet` (frontmatter) | Sàn sonnet cho subagent từ v0.22.0 |
| Agent `spec-reviewer` | `opus` (frontmatter); `fable` khi bật `ZNF_SPEC_REVIEW_MODEL` | Đọc lại spec bằng context mới trước khi bạn duyệt; xem [bên dưới](#spec-reviewer) |
| `gate` · các fork skill | `sonnet` | `gate`=drift cross-repo, miss âm thầm+đắt; fork skill ghim sonnet để không kế thừa session |
| `Explore` | **pin ≤ `sonnet` khi dispatch — KHÔNG để kế thừa Opus** | Tìm kiếm rộng: chạy search ở top tier là lãng phí thuần (`fix`/`gate` đã pin sẵn) |

## spec-reviewer

Agent `spec-reviewer` được brainstorming dispatch **một lần**, ở tier architectural, sau bước tự soát spec và trước khi bạn duyệt. Nó đọc spec trong context mới, đối chiếu với yêu cầu gốc của bạn, và chỉ báo: requirement thiếu hoặc mâu thuẫn, FR/SC không kiểm được, mâu thuẫn nội bộ, tên hoặc hợp đồng không có nguồn ground. Finding mức HIGH được đưa vào lời xin duyệt spec. Xem [agent `spec-reviewer`](/reference/agents/spec-reviewer).

Frontmatter ghim `model: opus` để seat Team Standard không cạn hạn mức vì Fable. Muốn soát spec bằng Fable, đặt `"ZNF_SPEC_REVIEW_MODEL": "fable"` trong khối `env` của `~/.claude/settings.json`; brainstorming đọc biến này và truyền `model: fable` khi dispatch. Biến không có hoặc mang giá trị khác thì chạy `opus`. Theo docs Claude Code, khi tổ chức chặn model qua allowlist `availableModels`, subagent chạy trên model kế thừa và phiên tương tác hiện một cảnh báo nêu cả hai tên model. Spec review không chặn: không có báo cáo thì brainstorming in `spec review: no report` rồi đi tiếp. Agent in dòng đầu `model: <tên model nó đang chạy>` và brainstorming in lại `spec review ran on: …`, nên bạn luôn thấy spec được soát bằng model nào.

## Advisor native

Advisor là tính năng của Claude Code ([tài liệu](https://code.claude.com/docs/en/advisor)): model chính hỏi một model mạnh hơn khi cần. **Kit không bật hộ** — mỗi người tự bật cho máy mình, bằng `/advisor opus` hoặc `"advisorModel": "opus"` trong `~/.claude/settings.json`.

Cặp model chính → advisor được phép:

| Model chính | Advisor |
|---|---|
| Opus 5 / Opus 5.5 | Fable hoặc Opus 5+ |
| Fable 5.1 | Chỉ Fable 5.1 |
| Sonnet 5.5 | Fable, Opus 5+, hoặc Sonnet 5.5 |

Lưu ý:

- Mỗi lần gọi, advisor đọc **cả transcript, không cache**, nên trừ vào quota của chính người bật; bật khi cần, đừng mặc định cho mọi phiên.
- Phiên đang chạy giữ advisor cũ cho tới khi `/compact` hoặc `/clear`.

## Route-log

[`zenify route-log`](/reference/cli/zenify_route-log) (`--since 30d`, `--json`) đọc các record skill ghi vào `.znf/route-log/` của checkout chính, ví dụ số liệu plan từ `zenify analyze`. Ghi best-effort, không bao giờ chặn. `zenify cost` in thêm token thinking của session chính và các mức effort đã gặp; file transcript không còn tồn tại (ví dụ symlink hỏng) được bỏ qua và số file bỏ qua được in ra, không làm lệnh crash.

## Đổi model

- **Xuống tier:** đặt `model: 'sonnet'` hoặc `'haiku'` (tham số dispatch nhận enum tier, không phải full ID). **Lên top tier:** với implementer, ghi `**Model:** opus` trong plan; ngoài các chỗ đã viết sẵn, **bỏ** tham số `model` để kế thừa session — đừng đặt alias `opus`, nó trôi sang bản mới nhất.
- **Effort quan trọng hơn tier** với code, nhưng chỉ với model chạy được nó: `claude-haiku-4-5` không hỗ trợ `xhigh` (bị âm thầm hạ effort, không báo lỗi). Nên **đừng ghép haiku với `xhigh`**; `xhigh` dành cho chỗ quy tắc ghi rõ (investigator vòng 3 trở lên, fix loop vòng 4–5).

## Ghi chú

- **Ghim full ID cho *session*, không dùng alias.** `opus` trỏ bản opus mới nhất và âm thầm override phiên bản đã ghim — với team, đó là cả team đổi hành vi một đêm mà không ai review. Full ID (trong `settings.json`) khóa hành vi session lại; nâng cấp là bump con số sau khi kiểm chứng qua gate + luồng thật. Còn **dispatch subagent** thì nhận *tier* từ quy tắc trong skill hoặc frontmatter — không ghim full ID ở đó.
- **Session ghim Opus 5.5 (1M context).** `zenify up` ghi `"model": "claude-opus-5-5[1m]"` vào `settings.json` của workspace (trước đó là `claude-opus-4-8`). [Hướng dẫn prompt Opus 5](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/prompting-claude-opus-5) của Anthropic nêu các mặc định cần để ý: dài hơn, tự verify khi không được yêu cầu, mở rộng scope ngoài yêu cầu, delegate nhiều hơn. Ghim full ID giữ cho team quyền đổi có kiểm chứng thay vì để alias tự quyết; một phiên muốn quay lại bản cũ dùng `/model claude-opus-4-8`. Máy không chạy `zenify up` (ví dụ worker zenie trong container) dùng `zenify ensure --workspace <dir>` để ghi cùng model pin, hook và `CLAUDE_CODE_SUBAGENT_MODEL`.
- **"Bỏ harness khi model mạnh" chỉ đúng với prompt thừa.** Anthropic tắt mặc định `TodoWrite`/`Task` trên model mới vì chúng tự track — đó là bỏ prompt, không phải bỏ kiến trúc. Gate xác định, route model rẻ, tái lập cả team, quan sát được: không phần nào là prompt scaffolding, nên giữ.

## Liên quan

- [Gate fail-open](/concepts/gate-fail-open) — các gate xác định là một nửa harness.
- [Ba lớp](/concepts/three-layers) — nơi các lựa chọn model sống.
- [Review và gate](/guides/review-and-gates) — tier review chọn model theo kích thước và blast radius.
- [Agent `spec-reviewer`](/reference/agents/spec-reviewer) · [`zenify route-log`](/reference/cli/zenify_route-log).

<!-- Nguồn (cho người bảo trì, không hiển thị):
Nội bộ: settings.json "model": claude-opus-5-5[1m]; znf/agents/{code-reviewer,scout,ui-verifier,researcher=sonnet; spec-reviewer=opus, fable khi ZNF_SPEC_REVIEW_MODEL=fable}; subagent-driven-development/SKILL.md + references/model-selection.md (dòng **Model:**, FAIL); fix/SKILL.md Step 1 (vòng 1-3); review/SKILL.md; brainstorming/SKILL.md (bước Spec review); cook/references/step6-implementation-notes.md (haiku không xhigh); internal/routelog.
Web: code.claude.com/docs/en/advisor (bảng pairing, cách bật, chi phí); anthropic.com/engineering/multi-agent-research-system (4×/15×); platform.claude.com/docs/.../prompting-claude-opus-5; revert path /model claude-opus-4-8.
-->
