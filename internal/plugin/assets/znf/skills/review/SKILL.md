---
name: review
description: Engine review hợp nhất của kit. Chọn tier cơ học theo diff, dispatch reviewer (T1 solo / T2 fan-out / T3 adversarial), trả finding theo schema chung. Cửa chính cho /review và cho ship step 5.
allowed-tools: Bash(git *) Bash(rg *) Bash(bash *) Bash(test *) Bash(awk *) Bash(zenify *) Agent Workflow
---

# znf:review — engine review hợp nhất

Một engine review DUY NHẤT. Standalone `/review`, và ship step 5 delegate vào đây.
Finding theo `_shared/finding-schema.md` (nguồn schema duy nhất — mọi tier cùng shape).

## Lifecycle 5 chốt (seam)

Engine chạy 5 chốt theo thứ tự. M4a chỉ làm REVIEW (3); 4 chốt kia là **stub inert**
(no-op, hành vi = như review hiện tại) và sẽ được các slice sau thay:

1. **PRE** — mechanical-gate (M4b, **live**). Chạy build/lint theo stack + anti-pattern scan CƠ HỌC trước khi tốn LLM; fail → short-circuit.
2. **BUNDLE** — smart-bundling diff lớn (M4c, **live**). ADDED>2000 → `zenify review-bundle` chia cụm-file (cap 600, tối đa 8), review per-bundle rồi gộp; ≤2000 giữ nguyên.
3. **REVIEW** — dispatch theo tier (phần thịt M4a, bên dưới).
4. **VERIFY** — finding-verifier cơ học `zenify review-verify` (M4b, **live**, mọi tier): bác finding có evidence không khớp file thật. T3 vẫn giữ adversarial-LLM bên trong workflow (chồng lên, kiểm việc khác).
5. **POST** — advisory (M4f) + learning-capture (M4e), **cả hai live**: gate `zenify review-advise-gate` quyết có gọi adviser read-only (`## Advisory`) không; rồi ghi record review vào store local `.znf/review-log/` qua `zenify review-log record` (best-effort). Cả hai KHÔNG đổi `shippable`.

> **Doctrine (M4d, live):** KHÔNG ở POST mà là lớp **dispatch-time** — sanitize `## Verified` (Bước 1b-doctrine) + tiêm preamble reviewer (Bước 3). Xem hai bước đó.

## Bước 1 — tính input cho tier (cơ học)

```bash
BASE=${BASE:-HEAD}            # ship truyền base; standalone dùng HEAD
ADDED=$(git diff --numstat "$BASE" | awk '{a+=$1+$2} END{print a+0}')
# shared contract: tái dùng tín hiệu của gate (DB collection/endpoint/queue/pub-sub)
SHARED=$(git diff "$BASE" | rg -c 'collection\(|@InjectModel|emit\(|publish\(|subscribe\(|\.route\(|router\.(get|post|put|delete)' >/dev/null && echo 1 || echo 0)
CRITICAL=0                    # caller (ship/user) set 1 nếu vùng nhạy cảm (auth/tenant/migration)
```

## Bước 1b — PRE mechanical-gate (short-circuit)

Chạy gate CƠ HỌC trước khi dispatch LLM. Khi được **ship** gọi (ship-pack có block `## Verified` xác nhận build/lint đã pass ở ship step 2), engine tự set `STATIC_OK=1` ngay trên dòng lệnh gate để tránh build/lint hai lần. Standalone `/review` (không có ship-pack) → để `STATIC_OK=0`, gate chạy full build/lint:

```bash
GATE=$(STATIC_OK=${STATIC_OK:-0} bash ~/.claude/skills/znf/skills/review/scripts/mechanical-gate "$BASE")
echo "$GATE"   # {"verdict":"pass|block","findings":[...]}
```

- `verdict=block` (build/lint fail hoặc conflict-marker) → **DỪNG**: đưa `findings` của gate vào report, `shippable:false`, in lý do dừng, KHÔNG dispatch REVIEW.
- `verdict=pass` → giữ `findings` cơ học (nếu có: focused-test/debugger) để gộp vào report cuối, rồi sang Bước 2.

## Bước 1b-doctrine — DOCTRINE sanitize ## Verified (no-claim, M4d)

Chỉ khi caller là **ship** (context có block `## Verified`). Chạy MỘT LẦN ở đây — trước MỌI nhánh dispatch (cả Bước 1c bundle lẫn Bước 2→3) — nên mọi reviewer sau đó đều thấy Verified đã sạch.

Lấy nội dung block `## Verified` từ ship-pack trong context, pipe qua subcommand:

```bash
printf '%s' "$VERIFIED_TEXT" | zenify review-doctrine   # {"verified":..,"stripped":[..]}
```

- Thay block `## Verified` trong context đưa reviewer bằng trường `.verified`.
- `.stripped[]` không rỗng → in lên report: "doctrine: đã gỡ N claim khỏi ## Verified: [...]".
- `zenify` vắng trên PATH, hoặc standalone `/review` (không có ship-pack) → **skip, no-op** kèm note "doctrine sanitize skipped". Fail-open: không bao giờ dừng review.

## Bước 1c — BUNDLE (chia diff lớn, seam BUNDLE — M4c)

Chỉ chạy khi `ADDED > 2000`. Diff nhỏ hơn (đại đa số) bỏ qua bước này, sang thẳng Bước 2 (select-tier trên NGUYÊN diff) như cũ.

```bash
if [ "$ADDED" -le 2000 ]; then
  :   # skip bundling — đi tiếp Bước 2 trên nguyên diff
elif ! command -v zenify >/dev/null 2>&1; then
  # bundler vắng (build cũ) → không bundle được → giữ hành vi cũ
  echo "diff > 2000 LOC nhưng review-bundle vắng → quá lớn, dừng (tách PR)"; exit 0
else
  PLAN=$(zenify review-bundle "$BASE")   # {"verdict":..,"bundles":[{id,loc,files}],"total_loc":X}
  echo "$PLAN"
fi
```

Xử lý theo `verdict` của `$PLAN`:

- `too-large` → **DỪNG**: in "quá lớn kể cả sau khi chia bundle (> 8 cụm) — tách PR rồi review lại", KHÔNG dispatch. `shippable:false`.
- `bundle` → review **per-bundle** rồi gộp:
  1. `MANIFEST=$(git diff --name-only "$BASE")` — danh sách path TẤT CẢ file đổi; truyền làm context cho MỌI bundle reviewer (để reviewer biết nửa kia của một contract có thể đổi ở bundle khác — chống mù cross-bundle).
  2. Với MỖI bundle `b` trong `PLAN.bundles`:
     - `ADDED_b = b.loc`
     - `SHARED_b` = tín hiệu shared-contract tính trên `git diff "$BASE" -- <b.files>` (cùng regex Bước 1).
     - `TIER_b` = `bash .../select-tier "$ADDED_b" "$SHARED_b" "$CRITICAL"` (in tier + lý do cho bundle này ra report).
     - Dispatch REVIEW theo `TIER_b` (Bước 3), phạm vi diff = `git diff "$BASE" -- <b.files>`, kèm `MANIFEST` làm context. Degrade-safe T3→T2 vẫn áp dụng per-bundle.
     - Thu `findings[]` của bundle.
  3. Gộp findings mọi bundle, dedup theo `title+file`.
  4. Bỏ qua Bước 2 (đã select-tier per-bundle) và đi tiếp **Bước 4** (VERIFY + POST) trên hợp findings.
- `passthrough` (không kỳ vọng khi ADDED>2000) → đi tiếp Bước 2 trên nguyên diff.

**Report phải in**: đã bundle mấy cụm, LOC + tier mỗi cụm, TRƯỚC khi dispatch — minh bạch như "in tier + lý do".

## Bước 2 — chọn tier (KHÔNG để LLM đoán)

Chạy script cơ học qua `bash` (file materialize ở 0o600, không có +x — luôn gọi bằng `bash`), đọc dòng đầu:

```bash
bash ~/.claude/skills/znf/skills/review/scripts/select-tier "$ADDED" "$SHARED" "$CRITICAL"
```

Dòng 1 = `T1|T2|T3`, dòng 2 = lý do. **In tier + lý do ra report** trước khi dispatch.

## Bước 3 — REVIEW dispatch theo tier

**Doctrine preamble (M4d):** đọc nguồn preamble một lần —
`DOCTRINE=$(awk '{print}' ~/.claude/skills/znf/skills/review/_shared/reviewer-doctrine.md 2>/dev/null)`
(file vắng → `DOCTRINE=""` + note "doctrine preamble unavailable"; fail-open). **Prepend `DOCTRINE` vào ĐẦU brief của MỌI reviewer** dispatch dưới đây — T1 solo, cả 5 agent T2 — và truyền `args.doctrine="$DOCTRINE"` cho Workflow T3. Điểm tiêm này dùng chung cho cả reviewer per-bundle ở Bước 1c.

- **T1 (solo):** dispatch 1 agent `code-reviewer` (template `requesting-code-review/code-reviewer.md`),
  model `sonnet` cho diff <50 LOC / mid cho phần còn lại. Trả `findings[]` theo schema chung.
- **T2 (fan-out):** dispatch song song (MỘT message) 5 agent, mỗi agent 1 chiều
  (bugs / security / perf / contracts / types), mỗi agent trả `findings[]` theo schema.
  Gộp, dedup theo `title+file`. KHÔNG dùng Workflow tool ở tier này.
- **T3 (adversarial):** kiểm tồn tại workflow trước:

  ```bash
  test -f "$HOME/.claude/skills/znf/workflows/review-changes.js" && echo present || echo missing
  ```

  - **present** → chạy Workflow tool `scriptPath: ~/.claude/skills/znf/workflows/review-changes.js`,
    `args: {diff: <git diff BASE..HEAD>, context: <ship-pack intent nếu có>, doctrine: <DOCTRINE>}`. Workflow tự
    fan-out + adversarial verify (3 skeptic, ≥2 confirm).
  - **missing** (teammate chưa `skills sync`, hoặc file bị xoá) → **degrade về T2** và ghi rõ
    trên report: "T3 degrade→T2: workflow vắng". KHÔNG gãy im lặng.

> Mọi finding có `file+line` PHẢI kèm `evidence` — trích **verbatim** MỘT dòng code lỗi (nguyên văn nội dung dòng trong file, KHÔNG kèm dấu `+`/`-` của diff) để `zenify review-verify` kiểm chứng; finding bịa dòng/quote sẽ bị bác ở VERIFY.

## Bước 4 — VERIFY (cơ học, mọi tier) + POST

VERIFY: gộp `findings[]` của REVIEW (mọi tier) rồi verify citation cơ học — bác finding có `evidence` không khớp file thật:

```bash
VERIFIED=$(printf '%s' "$FINDINGS_JSON" | zenify review-verify)   # {"findings":[kept],"kept":N,"refuted":M}
```

Nếu `command -v zenify` vắng → bỏ qua VERIFY kèm note "verify unavailable" (KHÔNG gãy, giữ nguyên findings). T3 vẫn giữ adversarial-LLM trong workflow.

```bash
KEPT_JSON=$(printf '%s' "${VERIFIED:-}" | jq -c '.findings' 2>/dev/null); { [ -z "$KEPT_JSON" ] || [ "$KEPT_JSON" = null ]; } && KEPT_JSON="$FINDINGS_JSON"   # kept sau VERIFY; fallback FINDINGS_JSON khi verify bị bỏ qua
```

POST: gộp findings kept + findings cơ học của gate (Bước 1b), rank theo severity, kết luận `shippable` (không CRITICAL/HIGH chưa xử lý).

POST-advisory (M4f, **live**): sau khi có `SHIPPABLE`, dựng `AdviseInput` rồi chạy gate cơ học quyết có gọi adviser không:

````bash
ADVISE_IN=$(printf '{"shared":%s,"critical":%s,"added":%s,"findings":%s,"shippable":%s}' \
  "$([ "$SHARED" = 1 ] && echo true || echo false)" \
  "$([ "$CRITICAL" = 1 ] && echo true || echo false)" \
  "${ADDED:-0}" "${KEPT_JSON:-[]}" "${SHIPPABLE:-false}")
ADVISE=$(printf '%s' "$ADVISE_IN" | zenify review-advise-gate 2>/dev/null)   # {"advise":..,"signals":[..]}
````

- `command -v zenify` vắng, gate lỗi, hoặc `.advise` != `true` → BỎ QUA adviser, report như cũ (KHÔNG block).
- `.advise == true` → chạy adviser (read-only, KHÔNG đổi shippable):
  1. Ghi file input adviser: `$KEPT_JSON` + `git diff --stat "$BASE"` + `SHIPPABLE` + `.signals` của gate.
  2. Dispatch `znf:code-reviewer` (Agent tool) với prompt = nội dung `_shared/adviser-prompt.md` + đường dẫn file input; override model tier sonnet.
  3. Trích đúng mục `## Advisory` từ output adviser, gắn vào report dưới nhãn "advisory — read-only, không ảnh hưởng shippable". CHỈ trích text `## Advisory`; BỎ mọi findings/verdict adviser lỡ trả — `shippable` KHÔNG đổi.
- Adviser vắng (chưa `zenify skills sync`) hoặc đi idle không trả report → report ghi "advisory skipped (adviser unavailable)", KHÔNG block, KHÔNG coi im lặng là sạch (CLAUDE.md §3).

POST-capture (M4e, **live**): cuối cùng, ghi lại review vào store local `.znf/review-log/` (main checkout) — **best-effort, KHÔNG chặn**:

````bash
command -v zenify >/dev/null && command -v jq >/dev/null && {
  REFUTED=$(printf '%s' "${VERIFIED:-}" | jq -r '.refuted // 0' 2>/dev/null); [ -n "$REFUTED" ] || REFUTED=0
  SIGNALS_JSON=$(printf '%s' "${ADVISE:-}" | jq -c '.signals // []' 2>/dev/null); { [ -n "$SIGNALS_JSON" ] && [ "$SIGNALS_JSON" != null ]; } || SIGNALS_JSON='[]'
  REC=$(printf '%s' "${KEPT_JSON:-[]}" | jq -c \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg repo "$(basename "$(git rev-parse --show-toplevel 2>/dev/null)" 2>/dev/null)" \
    --arg base "${BASE:-}" --arg head "$(git rev-parse --short HEAD 2>/dev/null)" \
    --arg tier "${TIER:-unknown}" --argjson refuted "$REFUTED" \
    --argjson shippable "${SHIPPABLE:-false}" --argjson signals "$SIGNALS_JSON" '
    { ts:$ts, repo:$repo, base:$base, head:$head, tier:$tier, outcome:"reviewed",
      findings:{ critical:([.[]|select(.severity=="CRITICAL")]|length),
                 high:([.[]|select(.severity=="HIGH")]|length),
                 medium:([.[]|select(.severity=="MEDIUM")]|length),
                 low:([.[]|select(.severity=="LOW")]|length) },
      kept:length, refuted:$refuted, shippable:$shippable, signals:$signals,
      categories:[.[].dimension] }' 2>/dev/null)
  [ -n "$REC" ] && printf '%s' "$REC" | zenify review-log record 2>/dev/null || true
}
```

- `zenify`/`jq` vắng, bất kỳ lệnh lỗi → BỎ QUA im lặng (`|| true`), review kết thúc bình thường. Capture KHÔNG đổi `shippable`, KHÔNG in report.
- Xem lại bằng `zenify review-log` (summary) hoặc `zenify review-log --json` (cho M6 sync).
````

## Report trả về

- tier đã chọn + lý do (+ "degrade→T2" nếu có)
- `findings[]` theo `_shared/finding-schema.md`, rank CRITICAL→LOW
- `shippable: true|false`
- `## Advisory` (M4f, nếu gate bật): 1–4 note read-only, KHÔNG ảnh hưởng `shippable`

## Ai gọi engine

- **Standalone `/review`** — review `git diff HEAD` (hoặc range chỉ định qua `BASE`).
- **ship step 5** — truyền `BASE` + ship-pack làm context; đưa CRITICAL/HIGH vào fix-loop của ship.

## Không có gì để review

Không phải git repo / diff rỗng → in "nothing to review" và dừng, không dispatch.
Diff cực lớn (>2000 LOC) → seam BUNDLE (Bước 1c) chia thành bundle cụm-file rồi review từng cụm. Chỉ dừng khi cần > 8 bundle (`verdict=too-large`) hoặc `review-bundle` vắng trên PATH — khi đó báo "quá lớn, tách PR".
