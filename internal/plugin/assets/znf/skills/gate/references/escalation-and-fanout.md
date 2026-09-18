# Escalation and fan-out rationale

Why the sweeps run as parallel agents, when a plain `/gate` pass is not enough, and how the
participant list is sourced.

## How the participant list is sourced

`zenify gate participants --json` lists the gate participants from two sources, merged by repo
name: first the knowledge store's `.config/gate-participants.json` (the team-maintained list,
which wins on a duplicate name), then every repo in the workspace whose `.claude/worktree.json`
declares `gate.sharedStore=true` and is not already listed. A repo can therefore appear with no
`worktree.json` flag of its own — that is the store speaking, not an error — and a repo that is
unexpectedly missing is added to the store file, not to this skill. Each entry carries
`accessPatterns` (the kinds of access that repo uses to reach the shared store — DI injection, a
model/registry symbol, the raw driver, whatever that repo's own config says) and `dbAccessor`
(the read-only tool/command for querying the real store from that repo, if one is configured).
Sweep against **this output**, not against any list written into this file — a repo that reacts
to *every* document change (a change-stream/CDC subscriber, if the workspace has one) is exactly
the kind of participant that goes missing from a hand-maintained list and breaks first when it
does.

## Why agents rather than inline

The intermediate volume here can be enormous — a registry pattern in one repo can fan out to
thousands of call sites — and once you have the `file:line` list, the match dumps are worthless.
Output that is a **map** delegates cleanly; output that is **evidence** does not, which is why
step 3 (verify against real data) stays inline instead of being dispatched.

## This is the default gate. When to escalate

`/gate` is what runs after every shared-resource edit: one inline pass, cheap, read-only, safe
to run unprompted. Use it by default and do not ask permission first.

Escalate to the global **`contract-sweep`** skill only when a single inline pass is not enough
to trust the answer:

- the sweep turns up more usages than you can hold in your head at once, so each one needs
  its own independent BREAKING / RISKY / SAFE verdict rather than one overall judgement
- the change spans several shared resources at the same time (a collection field *and* the
  queue payload that carries it, say)
- `/gate` came back clean but the change still feels wrong — a fresh agent per repo, with
  no memory of the edit, is not subject to the same blind spot

`contract-sweep` fans out one agent per repo and then verifies every usage individually, so it
costs real tokens and is `disable-model-invocation: true` — it must be invoked by hand. Both
skills draw the participant set from `zenify gate participants` and use the same three-pass
search, so they should never disagree; if they ever do, one of the two files has drifted and
needs fixing.
