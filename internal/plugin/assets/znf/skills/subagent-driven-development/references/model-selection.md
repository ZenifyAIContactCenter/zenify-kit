<!-- Moved verbatim from subagent-driven-development/SKILL.md § Model Selection (W4 slim-skills). Read when: choosing a tier for a dispatch, or wondering why the cheapest model is not always cheapest. -->

Use the least powerful model that can handle each role to conserve cost and increase speed.

**Mechanical implementation tasks** (isolated functions, clear specs, 1-2 files): use a fast, cheap model. Most implementation tasks are mechanical when the plan is well-specified.

**Integration and judgment tasks** (multi-file coordination, pattern matching, debugging): use a standard model.

**Architecture and design tasks**: use the most capable available model.
The standalone final whole-branch review is one of these.

**Review tasks**: choose the model with the same judgment, scaled to the
diff's size, complexity, and risk. A small mechanical diff does not need the
most capable model; a subtle concurrency change does. Scoped re-reviews of
small fix diffs take a cheap-to-mid tier.

**Implementer tier is mechanical — one feature: failures on the same test.**
`FAIL` — how many times the *same* test has failed for this task (ledger counts it).

```bash
ROUTE=$(bash ~/.claude/skills/znf/skills/_shared/scripts/select-route implementer FAIL=$FAIL); MODEL=$(printf '%s\n' "$ROUTE" | sed -n '1s/^model=//p')
```

default → sonnet · `FAIL=2` → opus with the error brief on disk
(the failing test, its output, the diff tried) · `FAIL=3` → `model=none`: stop dispatching
implementers, run `/fix` Step 1 with `select-route investigator ROUND=3` and come back with a
root cause. Escalate the *tier*, never the effort: no skill or agent declares `effort`.
Plans record decisions, not code, so an implementer always needs judgment: sonnet is the floor,
and haiku is no longer an implementer lane.

Record each implementer dispatch (best-effort):

```bash
command -v zenify >/dev/null && command -v jq >/dev/null && jq -nc --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg repo "$(basename "$(git rev-parse --show-toplevel 2>/dev/null)")" --arg branch "$(git branch --show-current 2>/dev/null)" \
  --arg fail "$FAIL" --arg model "$MODEL" --arg strong "${ZNF_STRONG_MODEL:-opus}" \
  '{ts:$ts,repo:$repo,branch:$branch,"site":"implementer",features:{FAIL:$fail},gates:(if ($fail|tonumber)>=3 then ["FAIL>=3"] elif ($fail|tonumber)==2 then ["FAIL=2"] else [] end),model:$model,strong:$strong}' \
  | zenify route-log record 2>/dev/null || true
```

**Name the model on every dispatch.** `zenify up` sets
`CLAUDE_CODE_SUBAGENT_MODEL=sonnet`, so a dispatch without `model` runs on
sonnet — right for mechanical work, wrong for design judgment. Pass `'opus'`
there; naming it keeps the intent visible.

**Turn count beats token price.** Wall-clock and context cost scale with how
many turns a subagent takes, and the cheapest models routinely take 2-3× the
turns on multi-step work — costing more overall. Use a mid-tier model as the
floor for reviewers and implementers.

`select-route` still accepts the code-spec key `SPEC` set to code (haiku) but no skill passes it; it is kept as the revert path.
