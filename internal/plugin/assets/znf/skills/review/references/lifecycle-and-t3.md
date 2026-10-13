<!-- Read when: you need what each of the four gates does end to end. -->

### The 4 gates, in full

The engine runs 4 gates in order:

1. **PRE** — `scripts/mechanical-gate` runs build/lint per stack plus a mechanical anti-pattern scan before any LLM budget is spent. `verdict=block` short-circuits: the report carries the gate findings and `shippable:false`, and REVIEW is not dispatched.
2. **BUNDLE** — only when ADDED > 2000: `zenify review-bundle` splits the diff into file-bundles (cap 600 LOC, max 8), each reviewed on its own and then merged. Smaller diffs pass through unchanged.
3. **REVIEW** — dispatch by tier (T1 solo, T2 five-dimension fan-out, T3 below), every reviewer brief prefixed with `_shared/reviewer-doctrine.md`. `ENGINE=codex` replaces the tiers with one codex pass.
4. **VERIFY** — `zenify review-verify` (every tier) rejects any finding whose `evidence` does not match the cited file. Kept findings merge with PRE's mechanical ones, rank by severity, and set `shippable` (no unresolved CRITICAL/HIGH).

### T3 dispatch, in full

- **T3:** the five T2 dimensions (bugs / security / perf / contracts / intent) dispatched in parallel in ONE message, all five on opus, each returning `findings[]`. Merge and dedup by `title+file` as in T2.
- There is no skeptic pass and no separate script: VERIFY is the only refutation step, and it runs on T3 findings exactly as on T1/T2.
- `Declined:` lines come from each reviewer's prose after its `findings[]`; the ship board carries them.
