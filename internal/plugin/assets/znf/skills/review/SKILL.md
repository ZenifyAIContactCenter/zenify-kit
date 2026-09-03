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
2. **BUNDLE** — smart-bundling diff lớn (M4c). M4a: T3 review nguyên khối.
3. **REVIEW** — dispatch theo tier (phần thịt M4a, bên dưới).
4. **VERIFY** — finding-verifier cơ học `zenify review-verify` (M4b, **live**, mọi tier): bác finding có evidence không khớp file thật. T3 vẫn giữ adversarial-LLM bên trong workflow (chồng lên, kiểm việc khác).
5. **POST** — learning-capture (M4e) + doctrine no-claim/anti-groupthink (M4d) + advisory (M4f). M4a: chỉ tổng hợp report.

## Bước 1 — tính input cho tier (cơ học)

```bash
BASE=${BASE:-HEAD}            # ship truyền base; standalone dùng HEAD
ADDED=$(git diff --numstat "$BASE" | awk '{a+=$1+$2} END{print a+0}')
# shared contract: tái dùng tín hiệu của gate (DB collection/endpoint/queue/pub-sub)
SHARED=$(git diff "$BASE" | rg -c 'collection\(|@InjectModel|emit\(|publish\(|subscribe\(|\.route\(|router\.(get|post|put|delete)' >/dev/null && echo 1 || echo 0)
CRITICAL=0                    # caller (ship/user) set 1 nếu vùng nhạy cảm (auth/tenant/migration)
```

## Bước 1b — PRE mechanical-gate (short-circuit)

Chạy gate CƠ HỌC trước khi dispatch LLM. Khi ship gọi (step 5 đã chạy lint/build), truyền `STATIC_OK=1` để gate chỉ quét anti-pattern:

```bash
GATE=$(STATIC_OK=${STATIC_OK:-0} bash ~/.claude/skills/znf/skills/review/scripts/mechanical-gate "$BASE")
echo "$GATE"   # {"verdict":"pass|block","findings":[...]}
```

- `verdict=block` (build/lint fail hoặc conflict-marker) → **DỪNG**: đưa `findings` của gate vào report, `shippable:false`, in lý do dừng, KHÔNG dispatch REVIEW.
- `verdict=pass` → giữ `findings` cơ học (nếu có: focused-test/debugger) để gộp vào report cuối, rồi sang Bước 2.

## Bước 2 — chọn tier (KHÔNG để LLM đoán)

Chạy script cơ học qua `bash` (file materialize ở 0o600, không có +x — luôn gọi bằng `bash`), đọc dòng đầu:

```bash
bash ~/.claude/skills/znf/skills/review/scripts/select-tier "$ADDED" "$SHARED" "$CRITICAL"
```

Dòng 1 = `T1|T2|T3`, dòng 2 = lý do. **In tier + lý do ra report** trước khi dispatch.

## Bước 3 — REVIEW dispatch theo tier

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
    `args: {diff: <git diff BASE..HEAD>, context: <ship-pack intent nếu có>}`. Workflow tự
    fan-out + adversarial verify (3 skeptic, ≥2 confirm).
  - **missing** (teammate chưa `skills sync`, hoặc file bị xoá) → **degrade về T2** và ghi rõ
    trên report: "T3 degrade→T2: workflow vắng". KHÔNG gãy im lặng.

> Mọi finding có `file+line` PHẢI kèm `evidence` — trích **verbatim** dòng code lỗi (nguyên văn) để `zenify review-verify` kiểm chứng; finding bịa dòng/quote sẽ bị bác ở VERIFY.

## Bước 4 — VERIFY (cơ học, mọi tier) + POST

VERIFY: gộp `findings[]` của REVIEW (mọi tier) rồi verify citation cơ học — bác finding có `evidence` không khớp file thật:

```bash
VERIFIED=$(printf '%s' "$FINDINGS_JSON" | zenify review-verify)   # {"findings":[kept],"kept":N,"refuted":M}
```

Nếu `command -v zenify` vắng → bỏ qua VERIFY kèm note "verify unavailable" (KHÔNG gãy, giữ nguyên findings). T3 vẫn giữ adversarial-LLM trong workflow.

POST: gộp findings kept + findings cơ học của gate (Bước 1b), rank theo severity, kết luận `shippable` (không CRITICAL/HIGH chưa xử lý).

## Report trả về

- tier đã chọn + lý do (+ "degrade→T2" nếu có)
- `findings[]` theo `_shared/finding-schema.md`, rank CRITICAL→LOW
- `shippable: true|false`

## Ai gọi engine

- **Standalone `/review`** — review `git diff HEAD` (hoặc range chỉ định qua `BASE`).
- **ship step 5** — truyền `BASE` + ship-pack làm context; đưa CRITICAL/HIGH vào fix-loop của ship.

## Không có gì để review

Không phải git repo / diff rỗng → in "nothing to review" và dừng, không dispatch.
Diff cực lớn (>~2000 LOC) vượt cả T3 → báo "quá lớn, chờ M4c smart-bundling" và dừng (M4a không tự bundle).
