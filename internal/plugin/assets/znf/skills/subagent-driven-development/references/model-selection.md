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

**Implementer tier is mechanical — plan specificity, then failures on the same test.**
Two features, both checkable without judgment:

- `SPEC=code` — every path in the task's `**Files:**` block appears inside a fenced code block
  of that same task (the plan carries the code; the implementer transcribes and tests).
  Otherwise `SPEC=prose`.
- `FAIL` — how many times the *same* test has failed for this task (ledger counts it).

```bash
ROUTE=$(bash ~/.claude/skills/znf/skills/_shared/scripts/select-route implementer SPEC=$SPEC FAIL=$FAIL); MODEL=$(printf '%s\n' "$ROUTE" | sed -n '1s/^model=//p')
```

`SPEC=code` → haiku · `SPEC=prose` → sonnet · `FAIL=2` → opus with the error brief on disk
(the failing test, its output, the diff tried) · `FAIL=3` → `model=none`: stop dispatching
implementers, run `/fix` Step 1 with `select-route investigator ROUND=3` and come back with a
root cause. Escalate the *tier*, never the effort: no skill or agent declares `effort`.

Record each implementer dispatch (best-effort):

```bash
command -v zenify >/dev/null && command -v jq >/dev/null && jq -nc --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg repo "$(basename "$(git rev-parse --show-toplevel 2>/dev/null)")" --arg branch "$(git branch --show-current 2>/dev/null)" \
  --arg spec "$SPEC" --arg fail "$FAIL" --arg model "$MODEL" --arg strong "${ZNF_STRONG_MODEL:-opus}" \
  '{ts:$ts,repo:$repo,branch:$branch,"site":"implementer",features:{SPEC:$spec,FAIL:$fail},gates:(if ($fail|tonumber)>=2 then ["FAIL=\($fail)"] else [] end),model:$model,strong:$strong}' \
  | zenify route-log record 2>/dev/null || true
```

**Name the model on every dispatch.** `zenify up` sets
`CLAUDE_CODE_SUBAGENT_MODEL=sonnet`, so a dispatch without `model` runs on
sonnet — right for mechanical work, wrong for design judgment. Pass `'opus'`
there and `'haiku'` for transcription; naming it keeps the intent visible.

**Turn count beats token price.** Wall-clock and context cost scale with how
many turns a subagent takes, and the cheapest models routinely take 2-3× the
turns on multi-step work — costing more overall. Use a mid-tier model as the
floor for reviewers and for implementers working from prose descriptions.
When the task's plan text contains the complete code to write, the
implementation is transcription plus testing: use the cheapest tier for
that implementer. Single-file mechanical fixes also take the cheapest tier.
