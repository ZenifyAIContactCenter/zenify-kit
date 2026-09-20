---
name: review
description: Unified review engine. Mechanically selects a tier from the diff, dispatches reviewers (T1 solo / T2 fan-out / T3 adversarial), returns findings per the shared schema. Main gate for /review and ship step 5.
allowed-tools: Bash(git *) Bash(rg *) Bash(bash *) Bash(test *) Bash(awk *) Bash(zenify *) Agent Workflow
---

# znf:review — unified review engine

One review engine: standalone `/review`, ship step 5 delegates here. Findings follow
`_shared/finding-schema.md` at every tier.

Five gates: PRE, BUNDLE, REVIEW, VERIFY, POST — one step each below (`references/lifecycle-and-t3.md`).
**Doctrine** is dispatch-time (Step 1b-doctrine + Step 3 preamble), not POST.

## Step 1 — compute tier input (mechanical)

```bash
BASE=${BASE:-HEAD}            # ship passes base; standalone uses HEAD
ADDED=$(git diff --numstat "$BASE" | awk '{a+=$1+$2} END{print a+0}')
# CODE lines only — a DELETION escalates; `.md` stays excluded or this file self-matches.
SHARED=$(git diff "$BASE" -- ':(exclude)*.json' ':(exclude)*.md' ':(exclude)*.lock' ':(exclude)*.snap' \
  | grep -E '^[+-]' | grep -vE '^(\+\+\+|---) ' \
  | rg -c 'collection\(|@InjectModel|emit\(|publish\(|subscribe\(|\.route\(|router\.(get|post|put|delete)' >/dev/null && echo 1 || echo 0)
CRITICAL=0                    # caller sets 1 for a sensitive area (auth/tenant/migration)
```

## Step 1b — PRE mechanical-gate (short-circuit)

```bash
GATE=$(STATIC_OK=${STATIC_OK:-0} bash ~/.claude/skills/znf/skills/review/scripts/mechanical-gate "$BASE")
echo "$GATE"   # {"verdict":"pass|block","findings":[...]}
```

`STATIC_OK=1` when **ship** calls (build/lint already passed); standalone `/review` leaves `0` so
the gate runs them.

- `verdict=block` (build/lint fail, conflict-marker) → **STOP**: gate `findings` into the report,
  `shippable:false`, print the reason, do NOT dispatch REVIEW.
- `verdict=pass` → keep the mechanical `findings`, go to Step 2.

## Step 1b-doctrine — DOCTRINE sanitize ## Verified (no-claim, M4d)

Only when **ship** calls, ONCE, before ANY dispatch:

```bash
printf '%s' "$VERIFIED_TEXT" | zenify review-doctrine   # {"verified":..,"stripped":[..]}
```

- Hand the reviewer `.verified` in place of the ship-pack's `## Verified`.
- `.stripped[]` non-empty → print "doctrine: stripped N claims from ## Verified: [...]".
- `zenify` missing, or standalone → **skip**, note "doctrine sanitize skipped"; fail-open.

## Step 1c — BUNDLE (split a large diff — M4c)

Only when `ADDED > 2000`; smaller diffs go to Step 2.

```bash
PLAN=$(zenify review-bundle "$BASE")   # {"verdict":..,"bundles":[{id,loc,files}],"total_loc":X}
```

Outcomes `missing` / `too-large` (STOP, split the PR) / `bundle` (per-bundle review, skip Step 2) / `passthrough` → `references/bundling.md` § Bundler outcomes.

**Report must print** cluster count and LOC + tier per cluster BEFORE dispatch.

## Step 2 — select tier (do NOT let the LLM guess)

Run via `bash` (materializes at 0o600, without +x), read line 1:

```bash
SELECT_TIER=$(bash ~/.claude/skills/znf/skills/review/scripts/select-tier "$ADDED" "$SHARED" "$CRITICAL")
TIER=$(printf '%s\n' "$SELECT_TIER" | sed -n '1p')   # reused by POST (Step 4)
printf '%s\n' "$SELECT_TIER"                          # print on the report
```

Line 1 = tier, line 2 = the reason. **Print tier + reason on the report** before dispatching.
Tier rule (script is the source): `CRITICAL=1` → T3 at any size; else T1 ≤200 LOC / T2 201–600 /
T3 >600, and `SHARED=1` **floors at T2** (T2's `contracts` goes to opus when shared).

## Step 3 — REVIEW dispatch by tier

**Doctrine preamble (M4d):** read it once —
`DOCTRINE=$(awk '{print}' ~/.claude/skills/znf/skills/review/_shared/reviewer-doctrine.md 2>/dev/null)`
(missing → `DOCTRINE=""` + note "doctrine preamble unavailable"; fail-open). **Prepend it to every
reviewer's brief** (T1, 5×T2, per-bundle) and pass `args.doctrine="$DOCTRINE"` to T3.

- **T1 (solo):** 1 `code-reviewer` agent (template in `subagent-driven-development/`), model
  `sonnet` under 50 LOC / mid otherwise. Returns `findings[]`.
- **T2 (fan-out):** 5 agents in parallel (ONE message), one dimension each (bugs / security / perf /
  contracts / types), each returning `findings[]`. **When `SHARED=1`, dispatch the `contracts` agent with model opus** (others stay default). Merge, dedup by `title+file`. No Workflow here.
**T3 reviewer model — mechanical, from the streak:**
```bash
STREAK=$(zenify review-log --json 2>/dev/null | jq -r --arg r "$(basename "$(git rev-parse --show-toplevel)")" --arg b "$(git branch --show-current)" \
  '[.[]|select(.repo==$r and .branch==$b)]|sort_by(.ts)|reverse|map(.shippable)|(index(true) // length)' 2>/dev/null); [ -n "$STREAK" ] || STREAK=0
ROUTE=$(bash ~/.claude/skills/znf/skills/_shared/scripts/select-route reviewer TIER="$TIER" BLOCKED_STREAK="$STREAK")
RMODEL=$(printf '%s\n' "$ROUTE" | sed -n '1s/^model=//p')
```
`RMODEL`≠`inherit` → `reviewModel:"$RMODEL"` in Workflow `args`, print route reason; then
`zenify route-log record` per `references/post-gates.md` § Route capture. `inherit` → nothing.

- **T3 (adversarial):** check first — `test -f "$HOME/.claude/skills/znf/workflows/review-changes.js" && echo present || echo missing`
  - **present** → run Workflow `scriptPath: ~/.claude/skills/znf/workflows/review-changes.js`,
    `args: {diff: <BASE..HEAD>, context: <ship-pack intent if any>, doctrine: <DOCTRINE>, reviewModel: <RMODEL or omitted>}`.
    Verifies **CRITICAL/HIGH only**; merge its `advisory[]` below confirmed findings (`references/lifecycle-and-t3.md`).
  - **missing** → **degrade to T2**, note "T3 degrade→T2: workflow missing". Never fail silently.

> Every finding with `file+line` MUST carry `evidence`: a **verbatim** quote of the offending line,
> without the `+`/`-` marker. A fabricated quote is refuted at VERIFY.

## Step 4 — VERIFY (mechanical, every tier) + POST

VERIFY: merge REVIEW's `findings[]`, reject any whose `evidence` mismatches the file:

```bash
VERIFIED=$(printf '%s' "$FINDINGS_JSON" | zenify review-verify)   # {"findings":[kept],"kept":N,"refuted":M}
```

`zenify` missing → skip VERIFY, note "verify unavailable" (findings kept).

```bash
KEPT_JSON=$(printf '%s' "${VERIFIED:-}" | jq -c '.findings' 2>/dev/null); { [ -z "$KEPT_JSON" ] || [ "$KEPT_JSON" = null ]; } && KEPT_JSON="$FINDINGS_JSON"   # falls back when VERIFY is skipped
```

POST: merge kept findings with Step 1b's mechanical ones, rank by severity, set `shippable` (no
unresolved CRITICAL/HIGH).

POST-advisory: build `AdviseInput`, gate it; `.advise == true` → dispatch `znf:code-reviewer`
(sonnet) with `_shared/adviser-prompt.md` + an input file, keep ONLY its `## Advisory`:

```bash
ADVISE=$(printf '%s' "$ADVISE_IN" | zenify review-advise-gate 2>/dev/null)   # {"advise":..,"signals":[..]}
```

POST-capture: record into `.znf/review-log/` — **best-effort, does NOT block**:

```bash
command -v zenify >/dev/null && [ -n "$REC" ] && printf '%s' "$REC" | zenify review-log record 2>/dev/null || true   # REC: tier, outcome, categories
```

Exact `ADVISE_IN`/`REC`: `references/post-gates.md`. Missing/failing zenify/jq/gate/adviser → skip
with a note, never block, never read silence as clean; `shippable` unchanged. Read back:
`zenify review-log --json`.

## Report returned

- selected tier + reason (+ "degrade→T2" if any)
- `findings[]` per `_shared/finding-schema.md`, ranked CRITICAL→LOW
- `shippable: true|false`
- `## Advisory` (gate on): 1–4 read-only notes, no effect on `shippable`

## Callers, and nothing to review

- **Standalone `/review`** — reviews `git diff HEAD` (or a range via `BASE`).
- **ship step 5** — passes `BASE` + ship-pack; feeds CRITICAL/HIGH into ship's fix-loop.
- Not a git repo / empty diff → print "nothing to review", stop, no dispatch.
- >2000 LOC goes through BUNDLE (Step 1c).

## References

- `references/bundling.md` — read when ADDED > 2000 and the diff must be split.
- `references/post-gates.md` — the exact advisory-gate and review-log snippets.
- `references/lifecycle-and-t3.md` — each gate end to end and the T3 workflow's rules.
