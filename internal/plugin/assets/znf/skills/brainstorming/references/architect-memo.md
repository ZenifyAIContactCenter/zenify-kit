<!-- Memo template for the architectural tier. Tier 2 (no gate fired): the session model writes it inline before the spec. Tier 3 (a select-route gate fired): the `architect` agent writes it to the memo path. Same ten sections either way — the discipline is shared; only the model is routed. Sections map onto the Brief: 2 → Goals/Non-goals, 3 → fields 2 and 7, 5 → fields 5, 6, 8. -->

# Architect memo — ten sections, in order (≤120 lines)

1. **Context and Scope** — the problem in two sentences; what is in scope, what is explicitly out.
2. **Goals and Non-Goals** — bullets; each non-goal carries a ceiling and a reopening trigger.
3. **The Actual Design** — 2–3 approaches, each with trade-offs on four axes: **operate /
   maintain / blast radius / cost to change later**. Name the chosen one.
4. **Alternatives Considered** — why each rejected approach lost, one line each.
5. **Cross-Cutting Concerns** — data ownership, deploy order, observability, rollback.
6. **Gates** (Spec Kit Phase −1) — one line each, `PASS` or `FAIL: <reason>`:
   - Simplicity Gate — ≤3 new components.
   - Anti-Abstraction Gate — framework used directly, one model per concept, no wrapper.
   - Integration-First Gate — contract and its test first; real service over mock.
7. **Innovation tokens** — list every technology/pattern new to this workspace; count them;
   >1 for one feature → justify in a paragraph or cut.
8. **Polyrepo checklist** — five answers, `N/A` written out where it does not apply:
   data + single writer · sync/async + consumer down · independent deploy + order ·
   the log/metric that shows breakage · timeout/retry only where a requirement needs it.
9. **Recommendation** — one paragraph; confidence `Strong | Worth exploring | Speculative`;
   `ADR conflict: <spec>` if it contradicts a planned/built spec, else `ADR conflict: none`;
   and the mechanical line `changed_decision: yes|no` (yes when the recommendation differs from
   the caller's approach sketch in what gets built or where).
10. **Deliberately NOT done** — each item you deliberately NOT built, with its reopening trigger (constitution P6).

Write it in the project's prose language (`_shared/artifact-style`); identifiers stay as-is.
