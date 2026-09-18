# Hotfix — why each rule exists

Rationale behind the steps in `SKILL.md`. Read when a step looks like overhead under time
pressure — every one of these was written because the shortcut cost more.

## Why diagnosis comes before the worktree

Creating a workspace before knowing the cause commits you to a forward fix before anyone has
decided that is the right response. A hotfix is written under time pressure and lands in
production, so it needs *more* diagnostic discipline than an ordinary bug fix, not less.
"It looked obvious" is the most expensive sentence available here.

## Why the response is the user's call

Deciding between revert, disable and fix forward is the highest-leverage decision in an incident.
Writing new code under pressure is when code is worst, while a revert adds no new risk. It is
also an action on production, which makes it the user's decision rather than the agent's.

If the forward fix turns out to need feature-sized work — several files, a contract change, a
design decision — that is itself the signal to revert instead and do it properly on the normal
feature base.

## Why the base ref is fetched before it is resolved

For a hotfix the base overrides the declared integration base to the latest release ref, so
resolving it against a stale local ref branches from the wrong release entirely. The tool's own
fetch is too late, because the ref is resolved before the tool runs. Full reasoning lives in
`znf:discipline` §8's base-ref rule.

`zenify hotfix baseref` reads that repo's own `.claude/worktree.json` (its `hotfix.baseStrategy`)
and resolves the concrete ref: a standalone/staging-strategy repo returns `staging`, a
`custom`-strategy repo returns its configured `hotfixBaseRef`, and a `release-latest`-strategy
repo scans `origin/release<N>` branches and returns the highest one. Which strategy each repo uses
is a project fact, not something to infer.

The ref is needed even when you never branch from it: the scout step and the gate step both depend
on knowing which code is actually running.

## Why `--type hotfix` is exempt from the one-task-per-repo guard

Production breaking mid-feature is the normal case. The hotfix worktree is allowed to exist
alongside the feature worktree already open, and it must, since it branches from a different base.

## Why the scout ref matters

The consumers that matter are the consumers of the version *currently running*, not of the repo's
normal feature base, which has moved on. Scouting the wrong ref produces a map that looks entirely
plausible and describes the wrong branch.

Target 4 (why the lines exist) carries the most weight here, because the bug is usually in code
someone else wrote. A global outage was traced to a refactor that had silently removed a CPU-time
guard — nobody knew why it was there.

## Why grounding matters more under production data

Two things beyond "does the field exist": **which values the field actually holds** — your new
branch may not cover a value that already exists in thousands of documents — and **which filter
every query must carry**. A missing tenant/scope filter returns correct-looking rows in
development, because dev data has one tenant, and most stores have no row-level security to catch
it.

## Why a clean check is not yet evidence

A positive result carries its own content; a *negative* one — "no match", "0 results", "OK",
"nothing found" — that lets you proceed does not, until a second independent mechanism agrees.
Five times in one session a tool reported clean and the clean was wrong.

## Why `/ship` runs unconditionally, even in a hurry

This is production, written fast, by a session that also verified its own work — the exact
single-control setup where **75.8%** of self-reported success claims were false, and where an LLM
judge asked to catch that reaches only AUROC 0.65. What closed the gap was an independent party
outside the agent's own control: **48% → 3%**. The reviewer inside `/ship` is that party. Being in
a hurry is the reason it is needed, not a reason to skip it.

## Why the board is `cat`ed rather than summarised

Every check that carries weight in a hotfix runs inside that gate, so `"/ship: ✅ all green"` hides
all of it — and this is the pipeline where that matters most. The per-check fingerprints are the
only part a reader can confirm without trusting the summary, and collapsing the board throws
exactly them away. Reading it from a file also means skipping the step leaves a missing tool call
rather than a sentence that reads fine either way.

(`at HEAD` was also wrong: `/ship` commits last, so the tree is dirty during every check and
`HEAD` is not what was tested — the fingerprint is.)
