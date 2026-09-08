---
name: review
description: The kit's unified review engine. Mechanically selects a tier based on the diff, dispatches reviewers (T1 solo / T2 fan-out / T3 adversarial), and returns findings per the shared schema. The main gate for /review and for ship step 5.
allowed-tools: Bash(git *) Bash(rg *) Bash(bash *) Bash(test *) Bash(awk *) Bash(zenify *) Agent Workflow
---

# znf:review — unified review engine

A SINGLE review engine. Standalone `/review`, and ship step 5 delegates into here.
Findings follow `_shared/finding-schema.md` (the single schema source — every tier shares the same shape).

## 5-gate lifecycle (seam)

The engine runs 5 gates in order. M4a implements only REVIEW (3); the other 4 gates are **inert stubs**
(no-op, behavior = same as the current review) and will be replaced by later slices:

1. **PRE** — mechanical-gate (M4b, **live**). Runs build/lint per stack + a MECHANICAL anti-pattern scan before spending any LLM budget; fail → short-circuit.
2. **BUNDLE** — smart-bundling for large diffs (M4c, **live**). ADDED>2000 → `zenify review-bundle` splits into file-bundles (cap 600, max 8), reviews per-bundle then merges; ≤2000 stays as-is.
3. **REVIEW** — dispatch by tier (the meat of M4a, below).
4. **VERIFY** — mechanical finding-verifier `zenify review-verify` (M4b, **live**, every tier): rejects findings whose evidence doesn't match the real file. T3 still keeps the adversarial-LLM inside the workflow (layered on top, checking something different).
5. **POST** — advisory (M4f) + learning-capture (M4e), **both live**: the `zenify review-advise-gate` gate decides whether to call the read-only adviser (`## Advisory`); then it records the review into the local store `.znf/review-log/` via `zenify review-log record` (best-effort). Neither changes `shippable`.

> **Doctrine (M4d, live):** NOT at POST but a **dispatch-time** layer — sanitizes `## Verified` (Step 1b-doctrine) + injects the reviewer preamble (Step 3). See those two steps.

## Step 1 — compute tier input (mechanical)

```bash
BASE=${BASE:-HEAD}            # ship passes base; standalone uses HEAD
ADDED=$(git diff --numstat "$BASE" | awk '{a+=$1+$2} END{print a+0}')
# shared contract: reuse the gate's signal (DB collection/endpoint/queue/pub-sub)
SHARED=$(git diff "$BASE" | rg -c 'collection\(|@InjectModel|emit\(|publish\(|subscribe\(|\.route\(|router\.(get|post|put|delete)' >/dev/null && echo 1 || echo 0)
CRITICAL=0                    # caller (ship/user) sets 1 if a sensitive area (auth/tenant/migration)
```

## Step 1b — PRE mechanical-gate (short-circuit)

Run the MECHANICAL gate before dispatching to the LLM. When called by **ship** (ship-pack has a `## Verified` block confirming build/lint already passed at ship step 2), the engine sets `STATIC_OK=1` directly on the gate command line to avoid running build/lint twice. Standalone `/review` (no ship-pack) → leaves `STATIC_OK=0`, the gate runs full build/lint:

```bash
GATE=$(STATIC_OK=${STATIC_OK:-0} bash ~/.claude/skills/znf/skills/review/scripts/mechanical-gate "$BASE")
echo "$GATE"   # {"verdict":"pass|block","findings":[...]}
```

- `verdict=block` (build/lint fail or conflict-marker) → **STOP**: put the gate's `findings` into the report, `shippable:false`, print the stop reason, do NOT dispatch REVIEW.
- `verdict=pass` → keep the mechanical `findings` (if any: focused-test/debugger) to merge into the final report, then move to Step 2.

## Step 1b-doctrine — DOCTRINE sanitize ## Verified (no-claim, M4d)

Only when the caller is **ship** (context has a `## Verified` block). Runs ONCE here — before ANY dispatch branch (both the Step 1c bundle and Step 2→3) — so every reviewer afterward sees an already-sanitized Verified.

Take the `## Verified` block content from the ship-pack in context, pipe it through the subcommand:

```bash
printf '%s' "$VERIFIED_TEXT" | zenify review-doctrine   # {"verified":..,"stripped":[..]}
```

- Replace the `## Verified` block in the context handed to the reviewer with the `.verified` field.
- `.stripped[]` non-empty → print on the report: "doctrine: stripped N claims from ## Verified: [...]".
- `zenify` missing from PATH, or standalone `/review` (no ship-pack) → **skip, no-op** with the note "doctrine sanitize skipped". Fail-open: never stops the review.

## Step 1c — BUNDLE (split a large diff, seam BUNDLE — M4c)

Only runs when `ADDED > 2000`. Smaller diffs (the vast majority) skip this step and go straight to Step 2 (select-tier on the WHOLE diff) as before.

```bash
if [ "$ADDED" -le 2000 ]; then
  :   # skip bundling — proceed to Step 2 on the whole diff
elif ! command -v zenify >/dev/null 2>&1; then
  # bundler missing (older build) → cannot bundle → keep old behavior
  echo "diff > 2000 LOC but review-bundle is missing → too large, stop (split the PR)"; exit 0
else
  PLAN=$(zenify review-bundle "$BASE")   # {"verdict":..,"bundles":[{id,loc,files}],"total_loc":X}
  echo "$PLAN"
fi
```

Handle based on `$PLAN`'s `verdict`:

- `too-large` → **STOP**: print "too large even after bundling (> 8 clusters) — split the PR then review again", do NOT dispatch. `shippable:false`.
- `bundle` → review **per-bundle** then merge:
  1. `MANIFEST=$(git diff --name-only "$BASE")` — the list of paths for ALL changed files; passed as context to EVERY bundle reviewer (so the reviewer knows the other half of a contract may have changed in a different bundle — guards against cross-bundle blindness).
  2. For EACH bundle `b` in `PLAN.bundles`:
     - `ADDED_b = b.loc`
     - `SHARED_b` = the shared-contract signal computed on `git diff "$BASE" -- <b.files>` (same regex as Step 1).
     - `TIER_b` = `bash .../select-tier "$ADDED_b" "$SHARED_b" "$CRITICAL"` (print the tier + reason for this bundle on the report).
     - Dispatch REVIEW by `TIER_b` (Step 3), diff scope = `git diff "$BASE" -- <b.files>`, with `MANIFEST` as context. Degrade-safe T3→T2 still applies per-bundle.
     - Collect the bundle's `findings[]`.
  3. Merge findings from every bundle, dedup by `title+file`.
  4. Skip Step 2 (tier already selected per-bundle) and proceed to **Step 4** (VERIFY + POST) on the union of findings.
- `passthrough` (not expected when ADDED>2000) → proceed to Step 2 on the whole diff.

**The report must print**: how many clusters were bundled, LOC + tier for each cluster, BEFORE dispatching — transparent like "print tier + reason".

## Step 2 — select tier (do NOT let the LLM guess)

Run the mechanical script via `bash` (the file materializes at 0o600, without +x — always invoke it with `bash`), read the first line:

```bash
SELECT_TIER=$(bash ~/.claude/skills/znf/skills/review/scripts/select-tier "$ADDED" "$SHARED" "$CRITICAL")
TIER=$(printf '%s\n' "$SELECT_TIER" | sed -n '1p')   # T1|T2|T3 — reused by POST learning-capture (Step 4)
printf '%s\n' "$SELECT_TIER"                          # still print tier + reason on the report
```

Line 1 = `T1|T2|T3`, line 2 = the reason. **Print tier + reason on the report** before dispatching.

## Step 3 — REVIEW dispatch by tier

**Doctrine preamble (M4d):** read the preamble source once —
`DOCTRINE=$(awk '{print}' ~/.claude/skills/znf/skills/review/_shared/reviewer-doctrine.md 2>/dev/null)`
(file missing → `DOCTRINE=""` + note "doctrine preamble unavailable"; fail-open). **Prepend `DOCTRINE` to the START of every reviewer's brief** dispatched below — T1 solo, all 5 T2 agents — and pass `args.doctrine="$DOCTRINE"` to the T3 Workflow. This injection point is shared with the per-bundle reviewers in Step 1c.

- **T1 (solo):** dispatch 1 `code-reviewer` agent (template `requesting-code-review/code-reviewer.md`),
  model `sonnet` for diff <50 LOC / mid for the rest. Returns `findings[]` per the shared schema.
- **T2 (fan-out):** dispatch 5 agents in parallel (ONE message), each agent covering 1 dimension
  (bugs / security / perf / contracts / types), each agent returns `findings[]` per the schema.
  Merge, dedup by `title+file`. Do NOT use the Workflow tool at this tier.
- **T3 (adversarial):** check the workflow exists first:

  ```bash
  test -f "$HOME/.claude/skills/znf/workflows/review-changes.js" && echo present || echo missing
  ```

  - **present** → run the Workflow tool `scriptPath: ~/.claude/skills/znf/workflows/review-changes.js`,
    `args: {diff: <git diff BASE..HEAD>, context: <ship-pack intent if any>, doctrine: <DOCTRINE>}`. The workflow
    handles its own fan-out + adversarial verify (3 skeptics, ≥2 confirm).
  - **missing** (teammate hasn't run `skills sync`, or the file was deleted) → **degrade to T2** and clearly note
    on the report: "T3 degrade→T2: workflow missing". Do NOT fail silently.

> Every finding with `file+line` MUST include `evidence` — a **verbatim** quote of ONE offending line of code (the exact line content in the file, WITHOUT the diff's `+`/`-` marker) so `zenify review-verify` can verify it; a finding that fabricates a line/quote will be rejected at VERIFY.

## Step 4 — VERIFY (mechanical, every tier) + POST

VERIFY: merge REVIEW's `findings[]` (every tier) then mechanically verify the citation — reject findings whose `evidence` doesn't match the real file:

```bash
VERIFIED=$(printf '%s' "$FINDINGS_JSON" | zenify review-verify)   # {"findings":[kept],"kept":N,"refuted":M}
```

If `command -v zenify` is missing → skip VERIFY with the note "verify unavailable" (does NOT fail, findings are kept as-is). T3 still keeps the adversarial-LLM inside the workflow.

```bash
KEPT_JSON=$(printf '%s' "${VERIFIED:-}" | jq -c '.findings' 2>/dev/null); { [ -z "$KEPT_JSON" ] || [ "$KEPT_JSON" = null ]; } && KEPT_JSON="$FINDINGS_JSON"   # kept after VERIFY; falls back to FINDINGS_JSON when verify is skipped
```

POST: merge the kept findings + the gate's mechanical findings (Step 1b), rank by severity, conclude `shippable` (no unresolved CRITICAL/HIGH).

POST-advisory (M4f, **live**): once `SHIPPABLE` is available, build `AdviseInput` then run the mechanical gate to decide whether to call the adviser:

````bash
ADVISE_IN=$(printf '{"shared":%s,"critical":%s,"added":%s,"findings":%s,"shippable":%s}' \
  "$([ "$SHARED" = 1 ] && echo true || echo false)" \
  "$([ "$CRITICAL" = 1 ] && echo true || echo false)" \
  "${ADDED:-0}" "${KEPT_JSON:-[]}" "${SHIPPABLE:-false}")
ADVISE=$(printf '%s' "$ADVISE_IN" | zenify review-advise-gate 2>/dev/null)   # {"advise":..,"signals":[..]}
````

- `command -v zenify` missing, gate errors, or `.advise` != `true` → SKIP the adviser, report as before (do NOT block).
- `.advise == true` → run the adviser (read-only, does NOT change shippable):
  1. Write the adviser input file: `$KEPT_JSON` + `git diff --stat "$BASE"` + `SHIPPABLE` + the gate's `.signals`.
  2. Dispatch `znf:code-reviewer` (Agent tool) with prompt = the content of `_shared/adviser-prompt.md` + the input file path; override the model tier to sonnet.
  3. Extract exactly the `## Advisory` section from the adviser's output, attach it to the report under the label "advisory — read-only, does not affect shippable". ONLY extract the `## Advisory` text; DISCARD any findings/verdict the adviser mistakenly returns — `shippable` does NOT change.
- Adviser missing (`zenify skills sync` not run) or goes idle without returning a report → the report notes "advisory skipped (adviser unavailable)", does NOT block, does NOT treat silence as clean (CLAUDE.md §3).

POST-capture (M4e, **live**): finally, record the review into the local store `.znf/review-log/` (main checkout) — **best-effort, does NOT block**:

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
} || true
````

- `zenify`/`jq` missing, any command errors → SKIP silently (`|| true`), the review ends normally. Capture does NOT change `shippable`, does NOT print on the report.
- Review it later with `zenify review-log` (summary) or `zenify review-log --json` (for M6 sync).

## Report returned

- the selected tier + reason (+ "degrade→T2" if any)
- `findings[]` per `_shared/finding-schema.md`, ranked CRITICAL→LOW
- `shippable: true|false`
- `## Advisory` (M4f, if the gate is on): 1–4 read-only notes, does NOT affect `shippable`

## Who calls the engine

- **Standalone `/review`** — reviews `git diff HEAD` (or a range specified via `BASE`).
- **ship step 5** — passes `BASE` + the ship-pack as context; feeds CRITICAL/HIGH into ship's fix-loop.

## Nothing to review

Not a git repo / empty diff → print "nothing to review" and stop, no dispatch.
An extremely large diff (>2000 LOC) → the BUNDLE seam (Step 1c) splits it into file-cluster bundles then reviews each cluster. Only stops when it would need > 8 bundles (`verdict=too-large`) or `review-bundle` is missing from PATH — in which case it reports "too large, split the PR".
</content>
