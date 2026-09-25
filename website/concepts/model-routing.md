---
title: Chọn model
---

# Chọn model: session, skill, subagent

Model được đặt ở **ba chỗ**, và đó là thứ quyết định cách đổi nó:

- **Session model** — vòng lặp chính, đặt bằng `/model`, ghim full ID trong `~/.claude/settings.json`. **Mọi skill chạy ở đây** và luôn dùng model này.
- **Subagent model** — agent dispatch qua Agent tool: ghim riêng (frontmatter, hoặc tham số `model`), hoặc kế thừa session khi không đặt.
- **Model theo route** — từ v0.26.0, năm điểm dispatch trong cook / fix / SDD / review / advisor không tự chọn tier nữa mà gọi script `select-route`; script đọc đặc điểm khách quan của task và trả về model. Model mạnh nhất của máy lấy từ env `ZNF_STRONG_MODEL`.

Một skill **không tự nâng model được**. Muốn một câu hỏi chạy trên model mạnh: gõ `/znf:advisor <câu hỏi>` (một consult, context mới, ghi log) thay vì đổi model cả session.

## Việc nào chạy model nào

Model mạnh điều phối, model rẻ làm phần song song — một agent tốn ~4× token so với chat, multi-agent ~15×, và Opus dẫn dắt + Sonnet subagent vượt Opus đơn lẻ ([multi-agent](https://www.anthropic.com/engineering/multi-agent-research-system)).

**Nguyên tắc (theo superpowers): dùng model *ít mạnh nhất mà vẫn kham được role*.** Với subagent, tier **không do người dispatch cảm nhận**: ở năm site bên dưới, `select-route` quyết định từ số liệu của task (số repo, có chạm contract chia sẻ, vòng thứ mấy, test fail bao nhiêu lần); các agent còn lại ghim tier trong frontmatter. Haiku hợp chỗ **miss thì rẻ** (transcription sai → fail test → fix loop bắt); KHÔNG hợp chỗ miss là false-negative *âm thầm và đắt* (review, chẩn đoán bug, drift cross-repo) — chỗ đó sàn sonnet.

| Nơi | Model | Vì sao |
|---|---|---|
| Agent `code-reviewer`, `scout`, `ui-verifier`, `researcher` | `sonnet` (frontmatter) | Sàn sonnet cho subagent từ v0.22.0; review nặng phán đoán đi qua tier T2/T3 bên dưới, không qua một agent đơn lẻ |
| Agent `architect` | `select-route architect` → model mạnh, hoặc không dispatch | Chỉ ở brainstorming tier **architectural** và khi một gate fire (`REPOS>=2`, `SHARED`, `NEW_CONTRACT`, `CRITICAL`); không gate nào fire thì viết memo inline trên session model. Một lần mỗi cook |
| `cook` bước 0–5 (brainstorm→analyze) | Session (Opus) | Quyết định thiết kế, chạy ở vòng lặp chính |
| `cook` bước 6 implementer (SDD) | `select-route implementer SPEC=… FAIL=…` | `SPEC=code` (plan chứa đủ code) → `haiku`; `SPEC=prose` → `sonnet`; cùng một test fail lần 2 → `opus` kèm brief lỗi trên đĩa; fail lần 3 → dừng implement, chạy investigator của `/fix` ở `ROUND=3` |
| `fix` investigator | `select-route investigator ROUND=…` | ROUND 1 `sonnet`; ROUND 2 (chẩn đoán đầu sai) `opus`, context mới; ROUND ≥ 3 model mạnh, kèm log và các giả thuyết đã loại |
| `review` T1 | `sonnet` <50 LOC / mid | Diff nhỏ dùng model rẻ; không xuống haiku — review haiku *grade tệ hơn* |
| `review` T2 fan-out | `sonnet`; `contracts` lên `opus` khi chạm contract chia sẻ | Sâu hơn ở phần rủi ro |
| `review` T3 | `security`+`contracts` `opus`; `bugs`/`perf`/`types` `sonnet`; verify `opus`. **Hai lần T3 liên tiếp không shippable** trên cùng branch → `select-route reviewer` đưa reviewer lên model mạnh | Blast radius auth/contract cần model mạnh; streak đọc từ `zenify review-log` |
| `/znf:advisor` | `select-route manual` → model mạnh | Cần cầu tay: người dùng gõ skill hoặc cụm kích hoạt; không tự chạy vì "thấy task khó" |
| `review` adviser · `gate` · các fork skill | `sonnet` | adviser=chất lượng lời khuyên; `gate`=drift cross-repo, miss âm thầm+đắt; fork skill ghim sonnet để không kế thừa session |
| `Explore` | **pin ≤ `sonnet` khi dispatch — KHÔNG để kế thừa Opus** | Tìm kiếm rộng: chạy search ở top tier là lãng phí thuần (`fix`/`gate` đã pin sẵn) |

## `select-route` — chọn model bằng đặc điểm task

Script duy nhất được phép chứa tên model mạnh: `~/.claude/skills/znf/skills/_shared/scripts/select-route <site> KEY=VAL…`. Site: `architect`, `investigator`, `implementer`, `reviewer`, `manual`. Kết quả ba dòng: `model=<haiku|sonnet|opus|<mạnh>|none|inherit>`, `reason: …`, `gates: …` (feature nào đã fire). Skill lấy nguyên dòng 1 làm tham số `model` của Agent và in dòng 2 lên báo cáo, nên người đọc luôn thấy vì sao model đó được chọn.

**Model mạnh đến từ env, không từ config kit và không dò account.** `select-route` đọc `ZNF_STRONG_MODEL`; giá trị hợp lệ là `fable` hoặc `opus`, mọi giá trị khác hoặc chưa đặt → `opus`, **không cảnh báo**. `zenify up` ghi `ZNF_STRONG_MODEL=opus` vào `env` của `~/.claude/settings.json` khi chưa có (không đè giá trị đã đặt); `zenify down` gỡ giá trị mặc định đó. Máy có tier mạnh hơn sửa tay một giá trị này. Nhờ vậy teammate dùng Pro không nhận dispatch tới model mà account không có, và không phải biết cơ chế này tồn tại.

`zenify rules lint` từ chối tên model mạnh viết trực tiếp trong `.md` của plugin (trừ description của skill `advisor`, nơi cụm kích hoạt cần chữ đó). Muốn dùng model mạnh ở một chỗ mới → thêm site vào `select-route`, không ghim tên.

## Route-log — số liệu để chỉnh ngưỡng

Mỗi lần `select-route` chạy trong cook / fix / SDD / review / advisor, skill ghi một record JSON vào `.znf/route-log/` của checkout chính (`site`, `features`, `gates`, `model`, `strong`, với architect thêm `changed_decision` = memo có đổi hướng thiết kế không). Ghi best-effort, không bao giờ chặn. Đọc bằng [`zenify route-log`](/reference/cli/zenify_route-log) (`--since 30d`, `--json`): gate nào fire mà architect gần như không đổi quyết định thì nới; `manual` bị gọi không có cụm kích hoạt thì siết description. `zenify cost` in thêm token thinking của session chính và các mức effort đã gặp để đối chiếu.

## Đổi model

- **Xuống tier:** đặt `model: 'sonnet'` hoặc `'haiku'` (tham số dispatch nhận enum tier, không phải full ID). **Lên top tier:** ở năm site route, đổi *feature* (ví dụ ghi `ROUND` đúng) chứ không đổi tên model; ngoài các site đó, **bỏ** tham số `model` để kế thừa session — đừng đặt alias `opus`, nó trôi sang bản mới nhất.
- **Effort quan trọng hơn tier** với code, nhưng chỉ với model chạy được nó: `claude-haiku-4-5` không hỗ trợ `xhigh` (bị âm thầm hạ effort, không báo lỗi). Nên **đừng ghép haiku với `xhigh`** — task transcription chạy haiku ở effort mặc định (đủ dùng), còn `sonnet` + `xhigh` để dành cho task làm từ prose nơi dial mạnh mới đáng tiền. Escalation của `select-route` đổi *tier*, không đổi effort: không skill hay agent nào khai `effort`.

## Ghi chú

- **Ghim full ID cho *session*, không dùng alias.** `opus` trỏ bản opus mới nhất và âm thầm override phiên bản đã ghim — với team, đó là cả team đổi hành vi một đêm mà không ai review. Full ID (trong `settings.json`) khóa hành vi session lại; nâng cấp là bump con số sau khi kiểm chứng qua gate + luồng thật. Còn **dispatch subagent** thì nhận *tier* từ `select-route` hoặc frontmatter — không ghim full ID ở đó.
- **Chưa lên Opus 5.** Không phải vì nó kém — Anthropic công bố ngang giá 4.8 và dẫn đầu benchmark hard-agentic ([opus-5](https://www.anthropic.com/news/claude-opus-5)). Nhưng chính [hướng dẫn prompt Opus 5](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/prompting-claude-opus-5) của Anthropic nêu các mặc định gây tranh cãi: dài hơn, tự verify khi không được yêu cầu, mở rộng scope ngoài yêu cầu, delegate nhiều hơn. Ghim giữ cho team quyền đổi có kiểm chứng thay vì để alias tự quyết.
- **"Bỏ harness khi model mạnh" chỉ đúng với prompt thừa.** Anthropic tắt mặc định `TodoWrite`/`Task` trên model mới vì chúng tự track — đó là bỏ prompt, không phải bỏ kiến trúc. Gate xác định, route model rẻ, tái lập cả team, quan sát được: không phần nào là prompt scaffolding, nên giữ.

## Liên quan

- [Gate fail-open](/concepts/gate-fail-open) — các gate xác định là một nửa harness.
- [Ba lớp](/concepts/three-layers) — nơi các lựa chọn model sống.
- [Review và gate](/guides/review-and-gates) — tier review chọn model theo kích thước và blast radius.
- [`/znf:advisor`](/reference/skills/advisor) · [agent `architect`](/reference/agents/architect) · [`zenify route-log`](/reference/cli/zenify_route-log).

<!-- Nguồn (cho người bảo trì, không hiển thị):
Nội bộ: settings.json "model": claude-opus-5-5[1m]; znf/agents/{code-reviewer,scout,ui-verifier,researcher=sonnet; architect=opus placeholder, model thật do select-route}; znf/skills/_shared/scripts/select-route (site/feature/threshold là nguồn của bảng); brainstorming/references/architect-gate.md (4 feature); subagent-driven-development/SKILL.md § Model Selection + references/model-selection.md (SPEC/FAIL); fix/SKILL.md Step 1 (ROUND); review/SKILL.md (BLOCKED_STREAK từ review-log); advisor/SKILL.md; internal/apply/strongmodel.go (EnsureStrongModelEnv/RemoveStrongModelEnv); internal/skilllint ScanStrongModel; internal/routelog; cook/references/step6-implementation-notes.md (haiku not xhigh-capable)
Web (fetched 2026-09-14, nguồn chính = Anthropic; tin cộng đồng chỉ làm màu):
- anthropic.com/news/claude-opus-5 (giá = 4.8, SOTA hard-agentic, "verifies its work and iterates")
- platform.claude.com/docs/.../prompting-claude-opus-5 (Anthropic tự nêu verbosity, over-verify, scope-expansion, over-delegation)
- anthropic.com/engineering/multi-agent-research-system (4×/15×; Opus-lead + Sonnet-subagents > single Opus; effort ladder 1/2-4/10+)
- claude.com/docs/.../todo-tracking (TodoWrite/Task off mặc định trên model mới)
- Reception thin/sentiment: HN 49079191 (chia hai phía); revert path /model claude-opus-4-8. KHÔNG verify: revert "4.7", tỉ lệ định lượng, Fable/Mythos model card, con số 80% cut.
-->
