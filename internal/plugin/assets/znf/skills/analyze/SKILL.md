---
name: analyze
description: Use to inspect a written spec+plan pair BEFORE implementing — mechanically checks requirement coverage (FR→task), leftover clarification markers, and Brief structure, then adds judgment on SC-testability, necessity, and DB-safety. advisory only, never blocks.
allowed-tools: Read Grep Bash(zenify analyze *)
---

# znf:analyze — inspect spec+plan before coding

**Announce:** "Using znf:analyze to inspect the spec+plan before implementation."

Checks a written spec+plan pair against `znf:_shared/constitution` (P1–P9) and
`znf:_shared/spec-template` (8-field Brief). **Advisory:** reports findings, does NOT block progress.
Two layers — mechanical (command) then judgment (skill).

## When to use

- After the spec **and** plan both exist, before dispatching the implementer (cook calls it here).
- Or invoke by hand: `/analyze <spec.md> <plan.md>` on any pair.

## Step 1 — mechanical scan (deterministic)

Run the command that reads coverage/marker/structural checks — this part must NOT be left to the LLM to count by hand:

```
zenify analyze --spec <spec-path> --plan <plan-path>
```

Read the output:
- **Coverage** — orphan FR (a requirement no task covers, CRITICAL), orphan task (a task that
  declares no `_Requirements:`, HIGH), dangling ref (plan cites an FR the spec doesn't have, HIGH).
- **Marker** — any leftover `[NEEDS CLARIFICATION` (HIGH), with line numbers.
- **Brief** — whether `## Brief` exists, and how many of the 8 fields are present.
- **Risk-metadata** — Brief missing the `_Blast-radius:` / `_DB:` / `_Rollback:` tag (HIGH per tag).

The command fails open: if it reports "could not analyze," note it and move on — don't treat it as a blocking error.

## Step 2 — four judgment passes (what the command can't do)

Read the spec+plan by eye and use judgment; each finding is severity **MEDIUM**:

1. **SC-testable (constitution P3).** Is each SC shaped as Given/When/Then, or an "the system
   shall" sentence that can actually be checked? An SC that's prose-only and not checkable → MEDIUM.
2. **Necessity (P6).** Does the Brief's "approach" field justify *which existing path can't already
   handle this* (necessity ladder)? Missing that justification when something new is being built → MEDIUM.
3. **db-3 + comprehension floor (P7).** Is the DB-guarantees block real (states query-plan / tenant-scope /
   keyset, or a justified N/A) or hand-waved? Does the spec describe the real flow + blast-radius before
   proposing any cut? Missing → MEDIUM.
4. **Risk-metadata substance (P9).** Do the three tags have real substance (the mechanical check only
   checks presence/emptiness — substance is this pass's job): does `_Blast-radius:` name the actual repos
   that would break; does `_DB:` cover every applicable concern (query-plan / tenant-scope / keyset) or
   carry a legitimate `N/A`; is `_Rollback:` feasible and does it name a prod-watch signal. Hand-waved → MEDIUM.

## Step 3 — report

Merge mechanical + judgment findings, sort by severity (CRITICAL → HIGH → MEDIUM), print concisely.
**Open the report with: "Advisory — does not block progress."** This skill has no authority to halt
the flow; it states findings so the user (or cook) can decide whether to fix the spec/plan or continue.
