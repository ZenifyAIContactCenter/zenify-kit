# Hotfix — why each rule exists

Rationale behind the steps in `SKILL.md`. Read when a step looks like overhead under time
pressure — every one of these was written because the shortcut cost more.

## How much diagnosis

This scales step 3; it never skips it. Confirming the root cause against real error output is
required on both paths.

- **Narrow** — one clear error, one obvious cause → one hypothesis, checked against the real
  error output, then confirm.
- **Wide** — cause unclear, multi-service, or hard to reproduce → `EnterPlanMode` (Opus) to run
  parallel hypotheses first, `ExitPlanMode` before deciding the response.

State which path and why (one line). If the narrow path's single hypothesis does not survive
confirmation, you were on the wide path all along.

## Why diagnosis comes before the worktree

Creating a workspace before knowing the cause commits you to a forward fix before anyone has
decided that is the right response. A hotfix is written under time pressure and lands in
production, so it needs *more* diagnostic discipline than an ordinary bug fix, not less.
"It looked obvious" is the most expensive sentence available here.

At step 3 use only the read-only rungs of the `/fix` loop: logs, `zenify db-read` / `redis-read`,
an existing failing test, a GET or HEAD request against a running environment. Anything with a side effect — a POST, a payload replay,
a job re-run — never touches production or a shared environment directly: it goes through a data
script run with `zenify data run`, dry-run first and the user's "ok" before a production write (see
`_shared/data-scripts.md`). With no read-only loop, confirm the cause
from the logs as before; `/fix`'s no-loop stop and its offer of temporary logging do not apply to
production. For a forward fix, the red command is built in the worktree after step 5 and must be
red before the step 7 fix. Scrub prod output before it reaches the conversation: secrets,
customer PII and tenant/account ids become `<REDACTED>`. `zenify data run` does not do this for
you — it masks credential values and hosts only (as `***`) — so a prod data script prints
record ids and counts only, never PII fields.

## Why the response is the user's call when the last deploy may be the cause

Deciding between revert, disable and fix forward is the highest-leverage decision in an incident.
Writing new code under pressure is when code is worst, while a revert adds no new risk. It is
also an action on production, which makes it the user's decision rather than the agent's —
whenever a revert is a real option. When the faulty commit predates the previous release tag,
reverting the last deploy would not remove the bug, so fix forward is the only response left and
step 4 takes it without asking.

If the forward fix turns out to need feature-sized work, that is itself the signal to revert
instead and do it properly on the normal feature base. "Feature-sized" has one definition, the
step 9 threshold: more than 3 changed files, or a shared resource reported by `/gate`. The count
runs before `/ship`, which is where the commit happens, so it diffs the working tree against the
merge-base with `$REF` and adds untracked files; `$REF...HEAD` would count nothing at that point.

## Why tags decide step 2 and step 4

Release tags (`vYYMMDD.n`, cut on the hotfix base) are the only machine-readable record of what
was released. Shell variables do not survive between the agent's Bash calls, so each block prints
its values and later steps reuse the printed text; an empty `$REF` would otherwise turn
`git show "$REF:<file>"` into a read of the index. Step 2 asks only when the newest tag is not on `$REF`; step 4 decides on its own
only when the faulty commit is an ancestor of the tag before it.

- **Sort by `-v:refname`, never by `creatordate`.** Release tags are lightweight (`git
  for-each-ref` reports `commit`), so `creatordate` is the commit date, and a tag placed on an
  older commit sorts in the wrong place. Version sort reads the name, which is the release order.
- **`C` comes from `git blame` on the one faulty line, not from `git log -S`.** `log -S
  --reverse | head -1` returns the oldest commit that changed how often the text appears in the
  file: a common line (`return null;`) or one removed and re-added in the last deploy resolves to
  an old commit and fix-forwards silently. It also stopped at the rename commit `9c0834242`
  instead of the original `927b43789` on a real renamed file. Blame returns the last commit that
  touched that exact line, follows whole-file renames, and needs no source text in a shell
  literal: the line number `N` comes from `$REF`'s copy via `grep -nF -f <needle>`. More than one
  match, or none, means the line cannot be pinned → ask; the `case` guard keeps an empty `N`
  from reaching `blame -L","`, which git reads as the whole file and answers with line 1's
  commit. Step 9 prints no number when `merge-base` fails (an empty or wrong ref), and no
  number means ask, never "3 or fewer". Blame errors only make `C` later (a
  reformat counts as a touch), so they only add a question.
- **What this check cannot see.** An old line that a newer commit elsewhere made reachable keeps
  an old `C`, so step 4 fix-forwards although reverting the last deploy would also end the
  incident. A fix forward is still a valid response there, only slower; the skill accepts it
  rather than guessing which other files are on the failing path.
- **Step 2 checks "the tag is on `$REF`", not "this is the build production runs".** The newest
  tag may not be deployed yet; checking the running version from telemetry is deferred.

## Why the base ref is fetched before it is resolved

For a hotfix the base overrides the declared integration base to the repo's declared hotfix base
(the ref production is cut from), so resolving it against a stale local ref branches from code
production is not running. The tool's own
fetch is too late, because the ref is resolved before the tool runs. Full reasoning lives in
`znf:discipline` §8's base-ref rule.

`zenify hotfix baseref` reads that repo's own `.claude/worktree.json` and resolves the concrete
ref through the same resolver `wt new --type hotfix` uses, so the two cannot disagree: a declared
`hotfixBaseRef` wins (`custom` requires one), and a standalone/staging-strategy repo returns its
`baseRef`. The old `release-latest` strategy now errors: it guessed the highest `origin/release<N>`
branch, and once a team stops cutting release branches the highest one is weeks stale while
looking current. Which ref each repo uses is a project fact, declared, never inferred.

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

## Clean checks

A clean check is not yet evidence. A positive result carries its own content; a *negative* one —
"no match", "0 results", "OK", "nothing found" — that lets you proceed does not, until a second
independent mechanism agrees. Five times in one session a tool reported clean and the clean was
wrong. Sweep with `rg`, not `grep -R`; confirm a wrapper tool's "not found" against the
underlying tool; on a connection failure run `nc -z <host> <port>` first.

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
