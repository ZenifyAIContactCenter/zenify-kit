<!-- Moved verbatim from review/SKILL.md § Step 4 POST-advisory + POST-capture (token-diet). Read when: running the advisory gate or recording the review into the local log — it holds the exact AdviseInput and record snippets. -->

### POST-advisory and POST-capture, in full

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
    --arg branch "$(git branch --show-current 2>/dev/null)" \
    --arg tier "${TIER:-unknown}" --argjson refuted "$REFUTED" \
    --argjson shippable "${SHIPPABLE:-false}" --argjson signals "$SIGNALS_JSON" '
    { ts:$ts, repo:$repo, base:$base, head:$head, branch:$branch, tier:$tier, outcome:"reviewed",
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

## Route capture (reviewer)

Only when `RMODEL` is not `inherit`:

```bash
command -v zenify >/dev/null && command -v jq >/dev/null && {
  jq -nc --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg repo "$(basename "$(git rev-parse --show-toplevel)")" \
    --arg branch "$(git branch --show-current)" --arg tier "$TIER" --arg streak "$STREAK" --arg model "$RMODEL" \
    --arg strong "${ZNF_STRONG_MODEL:-opus}" \
    '{ts:$ts,repo:$repo,branch:$branch,site:"reviewer",features:{TIER:$tier,BLOCKED_STREAK:$streak},gates:["BLOCKED_STREAK>=2"],model:$model,strong:$strong}' \
    | zenify route-log record 2>/dev/null || true
} || true
```
