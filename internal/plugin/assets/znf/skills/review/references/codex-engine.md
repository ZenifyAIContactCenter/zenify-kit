# Codex engine (Step 3, `ENGINE=codex`)

`ENGINE` comes from `zenify reviewer` (Step 2; empty → `agent`). Only `codex` takes this branch;
`agent` runs T1/T2/T3 as written in SKILL.md. `BASE` is Step 1's; `PACK` is the ship-pack path from
ship step 5 (empty standalone).

```bash
CODEX_JSON=$(zenify review-codex --base "$BASE" ${PACK:+--pack "$PACK"} 2>$TMPDIR/codex.err); RC=$?
```

- **exit 0** → `FINDINGS_JSON=$(printf '%s' "$CODEX_JSON" | jq .findings)` (`review-codex` prints
  `{"findings":[…]}`; Step 4's `review-verify` reads the bare array). Board line:
  `review … codex: <n> finding`, and the tier line reads `T3: một lượt codex`. Skip the tier dispatch.
- **exit ≠ 0** → board line `review … codex→agent: $(cat $TMPDIR/codex.err)`, then continue with
  T1/T2/T3 as if `ENGINE=agent`. Never fail silently.

Step 4 (VERIFY) is unchanged and runs on `FINDINGS_JSON` either way.
