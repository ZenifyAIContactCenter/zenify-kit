---
name: review
description: Unified review engine: selects a tier from the diff, dispatches reviewers (T1 solo / T2 fan-out / T3 five opus reviewers), returns findings per the shared schema. Main gate for /review and ship step 5.
allowed-tools: Bash(git *) Bash(rg *) Bash(bash *) Bash(test *) Bash(awk *) Bash(zenify *) Agent
---

# znf:review — unified review engine

One review engine: standalone `/review`, ship step 5 delegates here. Findings follow
`_shared/finding-schema.md`.

Four gates: PRE, BUNDLE, REVIEW, VERIFY — one step each below (`references/lifecycle-and-t3.md`).

## Step 1 — compute tier input (mechanical)

```bash
BASE=${BASE:-HEAD}            # ship: merge-base; standalone: HEAD (working tree)
PACK=${PACK:-}                # ship-pack path from ship step 5; empty standalone
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

`STATIC_OK=1` when **ship** calls (build/lint passed); standalone `/review` leaves `0` so the gate
runs them.

- `verdict=block` (build/lint fail, conflict-marker) → **STOP**: gate `findings` into the report,
  `shippable:false`, print the reason, do NOT dispatch REVIEW.
- `verdict=pass` → keep the mechanical `findings`, go to Step 2.

## Step 1c — BUNDLE (split a large diff)

Only when `ADDED > 2000`; smaller diffs go to Step 2.

```bash
PLAN=$(zenify review-bundle "$BASE")   # {"verdict":..,"bundles":[{id,loc,files}],"total_loc":X}
```

Outcomes `missing` / `too-large` (STOP, split the PR) / `bundle` (per-bundle, skip Step 2) / `passthrough` → `references/bundling.md` § Bundler outcomes.

**Report must print** cluster count, LOC + tier per cluster, BEFORE dispatch.

## Step 2 — select tier (do NOT let the LLM guess)

Run via `bash` (materializes at 0o600, without +x), read line 1:

```bash
SELECT_TIER=$(bash ~/.claude/skills/znf/skills/review/scripts/select-tier "$ADDED" "$SHARED" "$CRITICAL")
TIER=$(printf '%s\n' "$SELECT_TIER" | sed -n '1p')
printf '%s\n' "$SELECT_TIER"                          # print on the report
ENGINE=$(zenify reviewer 2>/dev/null | cut -d' ' -f1); [ -n "$ENGINE" ] || ENGINE=agent
```

Line 1 = tier, line 2 = the reason. **Print tier + reason on the report** before dispatching.
Tier rule: `CRITICAL=1` → T3 at any size; else T1 ≤200 LOC / T2 201–600 / T3 >600, and `SHARED=1`
**floors at T2** (T2's `contracts` goes to opus when shared).

## Step 3 — REVIEW dispatch by tier

`ENGINE=codex` → one codex pass replaces the tiers (`references/codex-engine.md`; a failure falls back to them).

**Doctrine preamble:** read it once —
`DOCTRINE=$(awk '{print}' ~/.claude/skills/znf/skills/review/_shared/reviewer-doctrine.md 2>/dev/null)`
(missing → `DOCTRINE=""` + note "doctrine preamble unavailable"; fail-open). **Prepend it to every
reviewer's brief** (T1, T2, T3, per-bundle).

- **T1 (solo):** 1 `code-reviewer` agent (template `subagent-driven-development/code-reviewer-template.md`),
  model `sonnet` under 50 LOC / mid otherwise. Returns `findings[]`.
- **T2 (fan-out):** 5 agents in parallel (ONE message), one dimension each (bugs / security / perf /
  contracts / intent), each returning `findings[]`. The `intent` agent also gets the ship-pack path `$PACK` (its `## Intent`); standalone with no pack it judges against the user's stated goal. **When `SHARED=1`, dispatch the `contracts` agent with model opus** (others stay default). Merge, dedup by `title+file`.
- **T3:** the five T2 dimensions in parallel (ONE message), all five on opus. No skeptic pass;
  VERIFY below still refutes fabricated citations.

> Every finding with `file+line` MUST carry `evidence`: a **verbatim** quote of the offending line,
> without the `+`/`-` marker. A fabricated quote is refuted at VERIFY.

## Step 4 — VERIFY (mechanical, every tier)

Merge REVIEW's `findings[]`, reject any whose `evidence` mismatches the file:

```bash
VERIFIED=$(printf '%s' "$FINDINGS_JSON" | zenify review-verify)   # {"findings":[kept],"kept":N,"refuted":M}
```

`zenify` missing → skip VERIFY, note "verify unavailable" (findings kept).

```bash
KEPT_JSON=$(printf '%s' "${VERIFIED:-}" | jq -c '.findings' 2>/dev/null); { [ -z "$KEPT_JSON" ] || [ "$KEPT_JSON" = null ]; } && KEPT_JSON="$FINDINGS_JSON"   # falls back when VERIFY is skipped
```

Then merge kept findings with Step 1b's mechanical ones, rank by severity, set `shippable` (no
unresolved CRITICAL/HIGH).

## Report returned

- selected tier + reason
- `findings[]` per `_shared/finding-schema.md`, ranked CRITICAL→LOW
- `shippable: true|false`

## Callers, and nothing to review

- **Standalone `/review`** — `git diff HEAD` (or a range via `BASE`); **ship step 5** — passes `BASE` + ship-pack, CRITICAL/HIGH feed its fix-loop.
- Not a git repo / empty diff → print "nothing to review", stop, no dispatch.

## References

- `references/bundling.md` — Step 1c.
- `references/lifecycle-and-t3.md` — the four gates end to end.
- `references/codex-engine.md` — the `ENGINE=codex` branch.
