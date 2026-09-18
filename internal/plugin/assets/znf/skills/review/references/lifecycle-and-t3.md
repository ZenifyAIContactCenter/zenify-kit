<!-- Moved verbatim from review/SKILL.md §§ 5-gate lifecycle, Step 3 T3 branch (token-diet). Read when: you need what each gate does end to end, or the T3 adversarial workflow's own fan-out and verify rules. -->

### The 5 gates, in full

The engine runs 5 gates in order. M4a implements only REVIEW (3); the other 4 gates are **inert stubs**
(no-op, behavior = same as the current review) and will be replaced by later slices:

1. **PRE** — mechanical-gate (M4b, **live**). Runs build/lint per stack + a MECHANICAL anti-pattern scan before spending any LLM budget; fail → short-circuit.
2. **BUNDLE** — smart-bundling for large diffs (M4c, **live**). ADDED>2000 → `zenify review-bundle` splits into file-bundles (cap 600, max 8), reviews per-bundle then merges; ≤2000 stays as-is.
3. **REVIEW** — dispatch by tier (the meat of M4a, below).
4. **VERIFY** — mechanical finding-verifier `zenify review-verify` (M4b, **live**, every tier): rejects findings whose evidence doesn't match the real file. T3 still keeps the adversarial-LLM inside the workflow (layered on top, checking something different).
5. **POST** — advisory (M4f) + learning-capture (M4e), **both live**: the `zenify review-advise-gate` gate decides whether to call the read-only adviser (`## Advisory`); then it records the review into the local store `.znf/review-log/` via `zenify review-log record` (best-effort). Neither changes `shippable`.

> **Doctrine (M4d, live):** NOT at POST but a **dispatch-time** layer — sanitizes `## Verified` (Step 1b-doctrine) + injects the reviewer preamble (Step 3). See those two steps.

### T3 dispatch, in full

- **T3 (adversarial):** check the workflow exists first:

  ```bash
  test -f "$HOME/.claude/skills/znf/workflows/review-changes.js" && echo present || echo missing
  ```

  - **present** → run the Workflow tool `scriptPath: ~/.claude/skills/znf/workflows/review-changes.js`,
    `args: {diff: <git diff BASE..HEAD>, context: <ship-pack intent if any>, doctrine: <DOCTRINE>}`. The workflow
    handles its own fan-out (security/contracts on opus, bugs/perf/types on sonnet) + adversarial
    verify of **CRITICAL/HIGH only** (3 opus skeptics, ≥2 confirm). MEDIUM comes back in `advisory[]`
    un-verified by LLM (the mechanical VERIFY below still runs on it); merge `advisory[]` into the
    findings passed to VERIFY, ranked below confirmed.
  - **missing** (teammate hasn't run `skills sync`, or the file was deleted) → **degrade to T2** and clearly note
    on the report: "T3 degrade→T2: workflow missing". Do NOT fail silently.
