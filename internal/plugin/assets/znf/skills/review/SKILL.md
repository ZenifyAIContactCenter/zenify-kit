---
name: review
description: The kit's unified review engine. Mechanically selects a tier based on the diff, dispatches reviewers (T1 solo / T2 fan-out / T3 adversarial), and returns findings per the shared schema. The main gate for /review and for ship step 5.
allowed-tools: Bash(git *) Bash(rg *) Bash(bash *) Bash(test *) Bash(awk *) Bash(zenify *) Agent Workflow
---

# znf:review — unified review engine

One review engine: standalone `/review`, and ship step 5 delegates here. Findings follow
`_shared/finding-schema.md` at every tier.

Five gates run in order: PRE, BUNDLE, REVIEW, VERIFY, POST — one step each below
(`references/lifecycle-and-t3.md`). **Doctrine** is dispatch-time, not POST: Step 1b-doctrine and
the Step 3 preamble.

## Step 1 — compute tier input (mechanical)

```bash
BASE=${BASE:-HEAD}            # ship passes base; standalone uses HEAD
ADDED=$(git diff --numstat "$BASE" | awk '{a+=$1+$2} END{print a+0}')
# CODE lines only, so a DELETION escalates; `.md` must stay excluded or this SKILL.md self-matches.
SHARED=$(git diff "$BASE" -- ':(exclude)*.json' ':(exclude)*.md' ':(exclude)*.lock' ':(exclude)*.snap' \
  | grep -E '^[+-]' | grep -vE '^(\+\+\+|---) ' \
  | rg -c 'collection\(|@InjectModel|emit\(|publish\(|subscribe\(|\.route\(|router\.(get|post|put|delete)' >/dev/null && echo 1 || echo 0)
CRITICAL=0                    # caller (ship/user) sets 1 if a sensitive area (auth/tenant/migration)
```

## Step 1b — PRE mechanical-gate (short-circuit)

```bash
GATE=$(STATIC_OK=${STATIC_OK:-0} bash ~/.claude/skills/znf/skills/review/scripts/mechanical-gate "$BASE")
echo "$GATE"   # {"verdict":"pass|block","findings":[...]}
```

`STATIC_OK=1` when **ship** calls (its `## Verified` means build/lint passed at step 2); standalone
`/review` leaves `0`, so the gate runs them.

- `verdict=block` (build/lint fail, conflict-marker) → **STOP**: gate `findings` into the report,
  `shippable:false`, print the reason, do NOT dispatch REVIEW.
- `verdict=pass` → keep the mechanical `findings` for the report, go to Step 2.

## Step 1b-doctrine — DOCTRINE sanitize ## Verified (no-claim, M4d)

Only when **ship** calls, ONCE, before ANY dispatch:

```bash
printf '%s' "$VERIFIED_TEXT" | zenify review-doctrine   # {"verified":..,"stripped":[..]}
```

- Hand the reviewer `.verified` in place of the ship-pack's `## Verified`.
- `.stripped[]` non-empty → print "doctrine: stripped N claims from ## Verified: [...]".
- `zenify` missing, or standalone `/review` → **skip**, note "doctrine sanitize skipped"; fail-open.

## Step 1c — BUNDLE (split a large diff — M4c)

Only when `ADDED > 2000`; smaller diffs go to Step 2.

```bash
PLAN=$(zenify review-bundle "$BASE")   # {"verdict":..,"bundles":[{id,loc,files}],"total_loc":X}
```

- `zenify` missing → print "diff > 2000 LOC but review-bundle is missing → too large, stop (split the PR)", stop.
- `too-large` → **STOP**: "too large even after bundling (> 8 clusters) — split the PR then review again", `shippable:false`.
- `bundle` → review per bundle, merge, dedup by `title+file`, skip Step 2, go to Step 4
  (`references/bundling.md`).
- `passthrough` → Step 2 on the whole diff.

**The report must print** the cluster count and LOC + tier per cluster BEFORE dispatch.

## Step 2 — select tier (do NOT let the LLM guess)

Run via `bash` (it materializes at 0o600, without +x), read line 1:

```bash
SELECT_TIER=$(bash ~/.claude/skills/znf/skills/review/scripts/select-tier "$ADDED" "$SHARED" "$CRITICAL")
TIER=$(printf '%s\n' "$SELECT_TIER" | sed -n '1p')   # T1|T2|T3 — reused by POST (Step 4)
printf '%s\n' "$SELECT_TIER"                          # print tier + reason on the report
```

Line 1 = `T1|T2|T3`, line 2 = the reason. **Print tier + reason on the report** before dispatching.
Tier rule (the script is the source): `CRITICAL=1` → T3 at any size; otherwise T1 ≤200 LOC / T2
201–600 / T3 >600, and `SHARED=1` **floors at T2** (T2's `contracts` goes to opus when shared).

## Step 3 — REVIEW dispatch by tier

**Doctrine preamble (M4d):** read it once —
`DOCTRINE=$(awk '{print}' ~/.claude/skills/znf/skills/review/_shared/reviewer-doctrine.md 2>/dev/null)`
(missing → `DOCTRINE=""` + note "doctrine preamble unavailable"; fail-open). **Prepend it to the
START of every reviewer's brief** — T1, the 5 T2 agents, per-bundle — and pass
`args.doctrine="$DOCTRINE"` to T3.

- **T1 (solo):** 1 `code-reviewer` agent (template `subagent-driven-development/code-reviewer-template.md`),
  model `sonnet` under 50 LOC / mid otherwise. Returns `findings[]`.
- **T2 (fan-out):** 5 agents in parallel (ONE message), one dimension each (bugs / security / perf /
  contracts / types), each returning `findings[]`. **When `SHARED=1`, dispatch the `contracts` agent with model opus** (the other 4 stay default). Merge, dedup by `title+file`. No Workflow here.
- **T3 (adversarial):** check it exists first —
  `test -f "$HOME/.claude/skills/znf/workflows/review-changes.js" && echo present || echo missing`
  - **present** → run the Workflow tool `scriptPath: ~/.claude/skills/znf/workflows/review-changes.js`,
    `args: {diff: <git diff BASE..HEAD>, context: <ship-pack intent if any>, doctrine: <DOCTRINE>}`.
    It verifies **CRITICAL/HIGH only**; merge its `advisory[]` below confirmed findings (`references/lifecycle-and-t3.md`).
  - **missing** → **degrade to T2**, note "T3 degrade→T2: workflow missing". Never fail silently.

> Every finding with `file+line` MUST carry `evidence`: a **verbatim** quote of ONE offending line, exactly as in the file, WITHOUT the `+`/`-` marker. A fabricated quote is refuted at VERIFY.

## Step 4 — VERIFY (mechanical, every tier) + POST

VERIFY: merge REVIEW's `findings[]`, reject any whose `evidence` doesn't match the file:

```bash
VERIFIED=$(printf '%s' "$FINDINGS_JSON" | zenify review-verify)   # {"findings":[kept],"kept":N,"refuted":M}
```

`zenify` missing → skip VERIFY, note "verify unavailable" (findings kept as-is).

```bash
KEPT_JSON=$(printf '%s' "${VERIFIED:-}" | jq -c '.findings' 2>/dev/null); { [ -z "$KEPT_JSON" ] || [ "$KEPT_JSON" = null ]; } && KEPT_JSON="$FINDINGS_JSON"   # falls back when VERIFY is skipped
```

POST: merge kept findings with the gate's mechanical ones (Step 1b), rank by severity, set
`shippable` (no unresolved CRITICAL/HIGH).

POST-advisory: build `AdviseInput` (`shared`, `critical`, `added`, `findings`, `shippable`), gate it,
and on `.advise == true` dispatch `znf:code-reviewer` (sonnet) with `_shared/adviser-prompt.md` and
an input file; take ONLY its `## Advisory`, drop any findings/verdict:

```bash
ADVISE=$(printf '%s' "$ADVISE_IN" | zenify review-advise-gate 2>/dev/null)   # {"advise":..,"signals":[..]}
```

POST-capture: record into `.znf/review-log/` — **best-effort, does NOT block**:

```bash
command -v zenify >/dev/null && [ -n "$REC" ] && printf '%s' "$REC" | zenify review-log record 2>/dev/null || true   # REC: tier, outcome, categories
```

Exact `ADVISE_IN`, `REC` and adviser input: `references/post-gates.md`. Anything missing or failing
(`zenify`, `jq`, gate error, `.advise` != `true`, adviser idle) → skip with a note; never block,
never read silence as clean. Neither POST step changes `shippable`. Read back: `zenify review-log --json`.

## Report returned

- the selected tier + reason (+ "degrade→T2" if any)
- `findings[]` per `_shared/finding-schema.md`, ranked CRITICAL→LOW
- `shippable: true|false`
- `## Advisory` (if the gate is on): 1–4 read-only notes, no effect on `shippable`

## Callers, and nothing to review

- **Standalone `/review`** — reviews `git diff HEAD` (or a range via `BASE`).
- **ship step 5** — passes `BASE` + the ship-pack; feeds CRITICAL/HIGH into ship's fix-loop.
- Not a git repo / empty diff → print "nothing to review" and stop, no dispatch.
- >2000 LOC goes through BUNDLE (Step 1c).

## References

- `references/bundling.md` — read when ADDED > 2000 and the diff must be split.
- `references/post-gates.md` — read for the exact advisory-gate and review-log snippets.
- `references/lifecycle-and-t3.md` — read for each gate end to end and the T3 workflow's rules.
