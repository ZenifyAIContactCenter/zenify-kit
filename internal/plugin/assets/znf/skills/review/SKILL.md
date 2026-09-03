---
name: review
description: Engine review hợp nhất của kit. Chọn tier cơ học theo diff, dispatch reviewer (T1 solo / T2 fan-out / T3 adversarial), trả finding theo schema chung. Cửa chính cho /review và cho ship step 5.
allowed-tools: Bash(git *) Bash(rg *) Agent Workflow
---

# znf:review — engine review hợp nhất

Một engine review DUY NHẤT. Standalone `/review`, và ship step 5 delegate vào đây.
Finding theo `_shared/finding-schema.md` (nguồn schema duy nhất — mọi tier cùng shape).

## Lifecycle 5 chốt (seam)

Engine chạy 5 chốt theo thứ tự. M4a chỉ làm REVIEW (3); 4 chốt kia là **stub inert**
(no-op, hành vi = như review hiện tại) và sẽ được các slice sau thay:

1. **PRE** — mechanical-gate (M4b). M4a: bỏ qua.
2. **BUNDLE** — smart-bundling diff lớn (M4c). M4a: T3 review nguyên khối.
3. **REVIEW** — dispatch theo tier (phần thịt M4a, bên dưới).
4. **VERIFY** — finding-verifier vs file thật (M4b). M4a: T3 giữ adversarial-LLM sẵn có; T1/T2 không verify.
5. **POST** — learning-capture (M4e) + doctrine no-claim/anti-groupthink (M4d) + advisory (M4f). M4a: chỉ tổng hợp report.

## Bước 1 — tính input cho tier (cơ học)

```bash
BASE=${BASE:-HEAD}            # ship truyền base; standalone dùng HEAD
ADDED=$(git diff --numstat "$BASE" | awk '{a+=$1+$2} END{print a+0}')
# shared contract: tái dùng tín hiệu của gate (DB collection/endpoint/queue/pub-sub)
SHARED=$(git diff "$BASE" | rg -c 'collection\(|@InjectModel|emit\(|publish\(|subscribe\(|\.route\(|router\.(get|post|put|delete)' >/dev/null && echo 1 || echo 0)
CRITICAL=0                    # caller (ship/user) set 1 nếu vùng nhạy cảm (auth/tenant/migration)
```

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

## Bước 4 — VERIFY / POST (stub M4a)

VERIFY: T3 đã verify trong workflow; T1/T2 chưa (M4b sẽ thêm). POST: tổng hợp `findings[]`,
rank theo severity, kết luận `shippable` (không CRITICAL/HIGH chưa xử lý).

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
