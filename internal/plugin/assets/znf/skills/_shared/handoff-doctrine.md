# Handoff doctrine

**Last updated:** 2026-09-08

Which kind of handoff to write, where it lives, and when it is retired. Downstream `znf:` skills and
any agent about to record session or milestone state read this at runtime; there is no cached copy to
bump (git history is the version). It is advisory — criteria to judge by, not a mechanism. It is the
twin of `znf:_shared/knowledge-doctrine`, which decides where a learned *rule* lives; this decides
where a *handoff* lives. Knowledge-doctrine defers handoff placement to here on purpose.

## Two kinds of handoff

A handoff records state so a later reader — a future session, or a teammate — can resume without
re-deriving it. There are exactly two kinds, and they differ on four axes: location, lifespan,
audience, mutability.

- **session-handoff** — an ephemeral note that carries one session's live state across a boundary.
  - *Location:* `docs/handoff/<dated>.md` (knowledge store).
  - *Lifespan:* ephemeral — it exists to survive compaction or a session/branch boundary, then is
    superseded once the work lands. Not permanent.
  - *Audience:* the next session (often yourself), mid-flight: what is done, what is in flight, what
    to do next, which traps were hit.
  - *Mutability:* mutable and disposable — overwrite or prune it freely; nothing downstream cites it.

- **milestone-record** — a committed record that a unit of work shipped.
  - *Location:* `m<N>-*.md` beside the ROADMAP (knowledge store).
  - *Lifespan:* permanent — it is a doc-site source, the durable record of what was built and why.
  - *Audience:* anyone later asking "what did milestone N ship, and what decisions stuck?" — not the
    mid-flight resumer.
  - *Mutability:* stable — edit only when the thing it records changes, like any other doc.

## Boundary triggers

The boundary you are crossing decides the kind.

- **compaction is imminent · the session is ending · a branch is being finished** → write a
  **session-handoff**. The state is live and mid-flight; the reader is the next session picking the
  work back up. `znf:finishing-a-development-branch` fires at exactly this point.
- **a milestone shipped by its definition-of-done** (merged, verified, the DoD met) → write a
  **milestone-record**. The state is settled; the reader is a future audit, not a resumer.

When both could apply — a session ends *because* a milestone shipped — write the milestone-record
(the durable one) and let the session-handoff retire into it (see Retirement).

## Reference, not copy

A handoff **points at** its sources — spec, plan, PR, memory, commit SHA — instead of copying their
content. This is the twin of promotion being a MOVE, not a COPY in `knowledge-doctrine`: a copied
spec paragraph or plan step drifts silently out of agreement with its source the moment the source
changes, and the handoff then asserts a stale version confidently. So cite the path and let the
reader open it; copy only the few facts that have no other home (a trap hit this session, a decision
made in chat that is not yet in any doc).

## Retirement

- **session-handoff:** retire it when the work it carried has landed — superseded by the
  milestone-record that records the shipped result, or pruned when it goes stale. It is scratch by
  design; nothing cites it, so removing it costs nothing. This doctrine does not run the prune — that
  is a doc-level delete, referenced here, not duplicated.
- **milestone-record:** does not retire on a schedule. Edit it only when its referent changes, like
  any other doc (git history is the version). Two records authoritative on one milestone is the same
  drift defect as two copies — fix by editing, not by adding a second.
