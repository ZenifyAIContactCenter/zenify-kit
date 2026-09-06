# Knowledge doctrine

**Last updated:** 2026-09-07

Where a learned thing lives, when it moves up, and when it is retired. Downstream `znf:` skills and
any agent deciding where to record something read this at runtime; there is no cached copy to bump
(git history is the version). It is advisory — criteria to judge by, not a mechanism. Detecting
promotion candidates automatically from review-log volume is a separate slice (M6d); handoff
placement (ephemeral session-handoff vs committed milestone-record) is another (M6e).

## The three layers

A learned thing is a fact or rule you did not have at the start of the session — read from a file or
the DB, or given as a correction. It lives in exactly one of three layers.

- **L1 — auto memory** (`.claude/memory/`, keyed per repo; type `user` / `feedback` / `project` /
  `reference`). Holds a fact or rule that changes a future decision and is specific to one repo or to
  the user. Read by relevance — recalled when something in the session matches its description. NOT
  cross-project: memory is keyed per repo, so a fact needed in another repo cannot be reached from
  here. Retired by `prune-memory`.

- **L2 — CLAUDE.md** (global `~/.claude/CLAUDE.md`; project `<repo>/CLAUDE.md`). Holds a standing
  working rule that must apply to every session (global) or every session in one repo (project). Read
  ambiently — loaded into every session's context, not recalled on demand. This is the layer for a
  rule memory cannot reach: something true across projects that per-repo memory would miss.

- **L3 — skill-rule / constitution** (`_shared/constitution.md` principles; a skill's `SKILL.md`
  body). Holds a rule that is procedural — it changes HOW a workflow runs — or universal to the kit's
  domain, and needs to be read at the point of a specific action rather than as ambient context. The
  constitution is the spec-discipline authority; a skill body is a step of a workflow.

## Placement test

Put a learned thing at the **lowest layer that still reaches every context where it must fire.** This
is the necessity ladder (constitution P6) applied to knowledge: do not place higher than needed. A
rule placed too high is asserted confidently on every match even after it goes stale — the same
failure as a stale memory, but worse, because a higher layer is read more often and trusted more.

Two axes decide the layer:

- **Scope** — one repo, recalled by relevance → L1 memory; one repo, must fire every session → L2
  project CLAUDE.md; every project → L2 global CLAUDE.md; the kit's own workflow → L3.
- **Trigger** — recalled by relevance (L1) · read ambiently every session (L2) · read at a specific
  action point (L3).

When the two axes disagree, scope decides the floor and trigger decides between L2 and L3.

## Promotion — moving a thing UP a layer

Promote when the lower layer no longer reaches every context the thing must fire in.

- **L1 memory → L2 CLAUDE.md:** when the same thing is needed in another repo, or the same correction
  recurs across projects. Signal: a `type: feedback` memory that is not repo-specific — memory cannot
  carry it across the repo boundary, so it belongs in global CLAUDE.md.

- **L2 CLAUDE.md → L3 skill-rule / constitution:** when the rule is really procedural (it belongs at
  an action point, not as ambient context), or when it has been stable and load-bearing enough that it
  should be enforced or checked rather than merely remembered. Signal: a CLAUDE.md rule that keeps
  being restated in skill dispatches.

**Promotion is a MOVE, not a COPY.** A rule must have exactly one authoritative home. Two copies are
the same defect as two memories authoritative on one subject: the loser is whichever one happens to be
read, and it drifts out of agreement silently. When you promote, delete the lower copy or reduce it to
a one-line pointer to the new home.

Detecting promotion candidates automatically — aggregating the review-log to surface a rule candidate
— is out of scope here; that is M6d.

## Retirement — taking a thing OUT

- **L1 memory:** retired by `prune-memory` (its KEEP / MERGE / FIX / DELETE / MOVE / SPLIT buckets).
  This doctrine does not duplicate those buckets — use that skill.

- **L2 / L3 (CLAUDE.md, skill-rule, constitution):** retire when the referent it names — a file,
  flag, command, or symbol — no longer exists, or when a newer rule contradicts it. Two rules
  authoritative on one subject is the same drift defect as two copies; fix it by editing the doc, like
  any other doc (git history is the version — there is no mechanism). Bias toward retiring
  aggressively at the higher layers: a stale rule there is asserted on every match and trusted, so it
  costs more than a stale memory does.
