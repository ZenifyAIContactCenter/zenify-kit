<!-- Read at the architectural tier, after clarify-lite and before the design step. Everything here is mechanical: the four features come from the grounding report and the user's request, the script decides, the skill obeys. -->

# Architect gate — tier 3 routing

**Features (write each as `NAME = value ← source` in the ledger):**

- `REPOS` — number of repos the request touches (from the grounding report's repo list).
- `SHARED` — 1 when the grounding report names a shared collection, pub/sub channel or HTTP
  contract another repo reads (`consumers` in another repo), else 0.
- `NEW_CONTRACT` — 1 when the request creates a new collection, channel, cross-service
  endpoint, hook, skill or agent, else 0.
- `CRITICAL` — 1 when a named path or keyword matches the critical set: `auth`, `tenant`,
  `billing`, `migration`, or a harness path `internal/plugin/assets/znf/{skills,agents,rules}/`,
  `internal/apply/`, `.claude/rules/`, `.claude/worktree.json`; else 0.

**Decide:**

```bash
ROUTE=$(bash ~/.claude/skills/znf/skills/_shared/scripts/select-route architect TIER=architectural REPOS=$REPOS SHARED=$SHARED NEW_CONTRACT=$NEW_CONTRACT CRITICAL=$CRITICAL)
AMODEL=$(printf '%s\n' "$ROUTE" | sed -n '1s/^model=//p'); GATES=$(printf '%s\n' "$ROUTE" | sed -n '3s/^gates: //p')
```

Print `$ROUTE` on the report.

**`none`** → tier 2: write the memo yourself, inline, from `architect-memo.md`; then the spec.

**Otherwise** → tier 3, once per brainstorm:

1. Write your 3-line approach sketch (what gets built, where, how it talks to the rest) to
   `${TMPDIR:-/tmp}/znf-architect-<slug>/sketch.md`.
2. Write `brief.md` beside it: absolute paths to problem + clarifications, `sketch.md`, the
   grounding report, `_shared/constitution.md`, `architect-memo.md`, the project
   context files (repo map, system map, glossary — whatever the project's convention names),
   and the memo output path `memo.md`.
3. `Agent(architect)` with `model` = `$AMODEL` verbatim, prompt = "Read the brief at <brief.md
   path>. Write the memo to the path it names. Return at most 40 lines." Ledger line names it.
4. Read `memo.md`; the spec's Approach, Blast-radius, Flow, Rollback and Non-goals come from it;
   the spec header's `**Nguồn:**` (or the project's equivalent) links the memo path.
5. Record — best-effort, never blocks (`CD` = the memo's `changed_decision:` value, `yes|no`):

```bash
command -v zenify >/dev/null && command -v jq >/dev/null && jq -nc \
  --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg repo "$(basename "$(git rev-parse --show-toplevel 2>/dev/null)")" \
  --arg branch "$(git branch --show-current 2>/dev/null)" --arg model "$AMODEL" --arg strong "${ZNF_STRONG_MODEL:-opus}" \
  --arg repos "$REPOS" --arg shared "$SHARED" --arg nc "$NEW_CONTRACT" --arg crit "$CRITICAL" --arg gates "$GATES" --arg cd "$CD" \
  '{ts:$ts,repo:$repo,branch:$branch,"site":"architect",features:{TIER:"architectural",REPOS:$repos,SHARED:$shared,NEW_CONTRACT:$nc,CRITICAL:$crit},gates:($gates|split(",")),model:$model,strong:$strong,changed_decision:$cd}' \
  | zenify route-log record 2>/dev/null || true
```

Dispatch error → record the same with `"outcome":"dispatch_error"`, `changed_decision` empty,
and continue as tier 2. Never a second dispatch in the same brainstorm.
