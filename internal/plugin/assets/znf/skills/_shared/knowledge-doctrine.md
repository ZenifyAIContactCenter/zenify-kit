# Knowledge doctrine

**Last updated:** 2026-09-09

Where a learned thing lives, when it moves up, and when it is retired. Downstream `znf:` skills and
any agent deciding where to record something read this at runtime; there is no cached copy to bump
(git history is the version). It is advisory — criteria to judge by, not a mechanism. Detecting
promotion candidates automatically from review-log volume is a separate slice (M6d); handoff
placement (ephemeral session-handoff vs committed milestone-record) is another (M6e).

## The layers

A learned thing is a fact or rule you did not have at the start of the session — read from a file or
the DB, or given as a correction. It lives in exactly one layer.

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

- **L4 — mechanical gate** (a `zenify` verb or a hook). Holds a norm that is both machine-checkable and
  load-bearing enough that "remembered" is not good enough — it is enforced at an action point rather
  than merely read. The highest layer; see Team-reach below for the fuller discussion.

## Team-reach — does this thing reach the whole team, or only me?

A second axis, orthogonal to scope. A learned thing lives in one of two reaches:

- **Personal reach** — `~/.claude/CLAUDE.md`, `~/.claude/rules/`, and auto memory
  (`.claude/memory/`). ALL of these reach only the session of the person who wrote them:
  memory is not synced, and `~/.claude` is per-machine. A binding TEAM norm parked here is
  invisible to teammates — they never had it, so they repeat the mistake it was meant to stop.
- **Distributed reach** — a rule form the kit ships to every teammate: an F1/F2 rule file under
  `.config/rules/` (delivered to each workspace `.claude/rules/` by `zenify config`), an
  L3 skill/constitution (delivered by `zenify skills sync`), or an L4 gate (delivered by the
  global-hooks channel). Only these actually reach the team.

**Memory is a flat store.** `.claude/memory/` is one flat, per-user set of working notes — no
`personal/` vs `shared/` split (a `shared/` subfolder does NOT reach the team; memory is not
synced, so it was theatre). Team-reach is achieved by **promoting** a note into a rule form,
never by a subfolder.

- **L4 — mechanical gate** — a deterministic check (a `zenify` verb or a hook) that ENFORCES a
  rule rather than asking the agent to remember it. The highest layer: read at an action point and
  fails the action on violation. Distributed by the global-hooks channel. Use it for a norm that is
  both machine-checkable and load-bearing enough that "remembered" is not good enough.

## Placement test

Put a learned thing at the **lowest layer that still reaches every context where it must fire.** This
is the necessity ladder (constitution P6) applied to knowledge: do not place higher than needed. A
rule placed too high is asserted confidently on every match even after it goes stale — the same
failure as a stale memory, but worse, because a higher layer is read more often and trusted more.

Three axes decide the layer:

- **Scope** — one repo, recalled by relevance → L1 memory; one repo, must fire every session → L2
  project CLAUDE.md; every project → L2 global CLAUDE.md; the kit's own workflow → L3.
- **Trigger** — recalled by relevance (L1) · read ambiently every session (L2) · read at a specific
  action point (L3) · enforced at an action point, not merely read (L4).
- **Team-reach** — does it need to reach only me, or the whole team? Personal reach stays at L1/L2
  personal CLAUDE.md; distributed reach needs a form the kit ships to every teammate — an F1/F2 rule
  file, an L3 skill/constitution, or an L4 gate.

When the axes disagree, scope decides the floor and trigger decides between L2 and L3. A norm that
must be enforced at write-time / is machine-checkable and load-bearing enough that "remembered" is
not good enough → L4, regardless of scope.

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

## Promotion order for a machine-checkable norm

When a norm CAN be checked by a machine, prefer the lower-ceremony form that still fires everywhere
it must: **F3 > F2 > F1** — an F3 gate (enforced) beats an F2 path-scoped rule (attached on Read of
a matching file) beats an F1 constitution line (ambient prose). Promote a `type: feedback` memory to
the right form, then **retire it from memory (MOVE, not a COPY)** so the rule has one home.

## Nominate → ratify — how a new rule enters

A rule is never created fully automatically (a false rule is asserted confidently forever — worse
than a missing one). An agent NOMINATES a candidate; a human RATIFIES it before it becomes a rule.

- Candidate is written to the synced store `reference/rule-candidates/<author>.md`, per-author.
- Ratify-gate: a candidate becomes a rule only when a reviewer flips its `Status: pending` to
  `ratified` and promotes it into F1/F2/F3. The agent does not create a rule from a candidate.

## Retirement — taking a thing OUT

- **L1 memory:** retired by `prune-memory` (its KEEP / MERGE / FIX / DELETE / MOVE / SPLIT buckets).
  This doctrine does not duplicate those buckets — use that skill.

- **L2 / L3 (CLAUDE.md, skill-rule, constitution):** retire when the referent it names — a file,
  flag, command, or symbol — no longer exists, or when a newer rule contradicts it. Two rules
  authoritative on one subject is the same drift defect as two copies; fix it by editing the doc, like
  any other doc (git history is the version — there is no mechanism). Bias toward retiring
  aggressively at the higher layers: a stale rule there is asserted on every match and trusted, so it
  costs more than a stale memory does.
