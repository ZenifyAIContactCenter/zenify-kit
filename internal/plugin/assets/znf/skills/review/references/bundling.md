<!-- Moved verbatim from review/SKILL.md § Step 1c BUNDLE (token-diet). Read when: ADDED > 2000 and the diff has to be split into bundles. -->

### Step 1c — BUNDLE (split a large diff, seam BUNDLE — M4c)

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
