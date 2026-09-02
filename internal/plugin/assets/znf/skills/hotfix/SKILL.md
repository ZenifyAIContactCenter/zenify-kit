---
name: hotfix
description: Handle a live production bug in a polyrepo workspace — diagnose first, then decide the response with the user (revert, disable, or fix forward), and only for a forward fix create an isolated worktree branched from the repo's configured hotfix base ref (never the default feature base). Scouts what depends on the code, verifies, gates and ships. Commits + pushes the hotfix branch once verified, but never creates PRs or merges. User-invoked only (whether something is a hotfix is the user's urgency call). If a bug looks live-critical, you may SUGGEST running /hotfix, but don't run it automatically.
disable-model-invocation: true
argument-hint: "[short-kebab-desc]"
allowed-tools: Bash(git *) Bash(wt *) Bash(zenify *) Read Grep Bash(rg *) Bash(cat *) Agent
---

Hotfix: **$ARGUMENTS**

## How much diagnosis

This scales the diagnosis in step 3; it never skips it. Confirming the root cause against real
error output is required on both paths — this lands in production, where "it looked obvious" is
the most expensive sentence available.

**Narrow** — one clear error pointing at one obvious cause (a typo, an inverted condition, a
missing field) → one hypothesis, checked against the real error output, then confirm.

**Wide** — root cause unclear, multi-service, or hard to reproduce → `EnterPlanMode` (Opus) to
run parallel hypotheses first, `ExitPlanMode` before deciding the response.

State which path and why (one line). If the narrow path's single hypothesis does not survive
confirmation, you were on the wide path all along.

## Steps

1. **Affected repo(s):** decide which repo(s) the live bug is in (route per the workspace's own
   CLAUDE.md, if it has one). If unclear, locate it first — don't guess.

2. **Fetch first, then resolve the hotfix base ref** for each affected repo, then **confirm with
   the user** (deploy-critical — the base ref differs per repo, and is never guessed from a
   pattern here):
   ```bash
   git -C <repo> fetch origin              # BEFORE resolving the base ref — see below
   REF=$(zenify hotfix baseref <repo>)
   echo "$REF"                              # confirm with the user that this is what is live
   ```
   `zenify hotfix baseref` reads that repo's own `.claude/worktree.json` (its
   `hotfix.baseStrategy`) and resolves the concrete ref for you — a standalone/staging-strategy
   repo returns `staging`, a `custom`-strategy repo returns its configured `hotfixBaseRef`, and a
   `release-latest`-strategy repo scans `origin/release<N>` branches and returns the highest one.
   Which strategy each repo uses is a project fact, not something to infer here.

   **The fetch order matters and getting it wrong is silent.** Remote-tracking refs (including
   any `origin/release*` branches) are only as current as the last fetch. `wt`'s own fetch (on
   recent builds) runs after you have already resolved and passed `--base`, so it cannot help here —
   you must fetch yourself before resolving the release ref.
   Resolve the base ref before fetching and you can miss a release cut this morning — then
   branch, scout, and fix against the wrong version, with everything looking correct the whole
   way.

   You need this ref now even if you never branch from it: step 4 and step 6 both depend on
   knowing which code is actually running.

3. **Diagnose — before touching anything.** A hotfix is written under time pressure and lands in
   production, so it needs *more* diagnostic discipline than an ordinary bug fix, not less. Run
   **`/fix` steps 0-2** rather than improvising: ground in the real error output first (logs,
   stacktrace, `docker logs <container>`), then hypotheses, then confirm the root cause. If you
   cannot confirm it, say so — **do not guess-fix production.**

   This comes before the worktree on purpose. Creating a workspace before knowing the cause
   commits you to a forward fix before anyone has decided that is the right response.

4. **Decide the response — the user's call, not yours.** With the cause confirmed, there are
   three responses and forward-fixing is only one of them:

   ```
   ▸ REVERT the deploy or commit that introduced it   — usually fastest and safest;
                                                         removes the new code instead of
                                                         adding more under time pressure
   ▸ DISABLE the path (feature flag / kill switch)    — if that path has one
   ▸ FIX FORWARD                                      — only when neither above applies,
                                                         or the bug predates the last deploy
   ```

   Present the three with a recommendation and **wait**. This is an action on production, so it
   is the user's decision; and it is the highest-leverage decision in an incident — writing new
   code under pressure is when code is worst, while a revert adds no new risk.

   **If the answer is revert or disable: stop here.** Do the revert or the flag change, verify
   the incident is over, and skip to step 9 — no worktree, no scout, no fix. Then plan the real
   fix calmly afterwards, on whatever base that repo normally branches feature work from.

   **If the forward fix turns out to need feature-sized work** (several files, a contract
   change, a design decision), that is itself the signal to **revert instead** and do it
   properly on the normal feature base. Do not run `/cook` on a hotfix base ref.

5. **Create the hotfix worktree** — forward fix only, and always a worktree (house rule #8). Use
   the `$REF` you resolved **after fetching** in step 2; same slug across repos so the branch name
   matches:
   ```bash
   cd <repo> && wt new $ARGUMENTS --type hotfix --base "$REF"
   ```
   If more than one repo is affected, one worktree in each. Do not re-derive `$REF` here without
   fetching and re-running `zenify hotfix baseref` — the value from step 2 is the one the user
   confirmed.
   `--type hotfix` is deliberately **exempt** from the one-task-per-repo-per-session guard, because
   production breaking mid-feature is the normal case: this worktree is allowed to exist alongside
   the feature worktree already open, and it must, since it branches from a different base.
   This leaves the main checkout untouched and gives each repo its own worktree at
   `<repo>/.worktrees/$ARGUMENTS`, its own port, its `.env` seeded, and dependencies linked — so
   the app can actually be run to verify the fix in step 8. Do the rest of this flow inside that
   worktree path, not the main checkout.

6. **`Skill(scout)` — and scout the RESOLVED ref, not the default feature base.** Invoke it as a
   tool, not as an intention: `Skill(scout)` and the `Agent(scout)` it dispatches each leave a
   line carrying their name, so a skipped step is visible instead of having to be taken on trust.
   Find what depends on the code you are about to change: consumers, the tests that cover it,
   anything written in the same operation, and why those lines exist (`git blame -L`, `git log
   -S`).

   **The ref matters and getting it wrong fails silently.** The consumers that matter are the
   consumers of the version *currently running* — the `$REF` resolved in step 2 — not of the
   repo's normal feature base, which has moved on. Scouting the wrong ref produces a map that
   looks entirely plausible and describes the wrong branch. State which ref you scouted.

   Target 4 (why the lines exist) carries the most weight here, because the bug is usually in
   code someone else wrote. A global outage was traced to a refactor that had silently removed a
   CPU-time guard — nobody knew why it was there.

7. **Fix.** Read the real code first, smallest diff, and do not refactor anything you are not
   fixing.

   **Call `Skill(ground)` on anything you have not verified this session** — not just "does the field
   exist" but the two things that matter under production data: **which values the field
   actually holds** (a distinct-values query against the workspace's own read-only DB accessor —
   your new branch may not cover a value that already exists in thousands of documents), and
   **which filter every query here must carry** (a missing tenant/scope filter returns
   correct-looking rows in development, because dev data has one tenant, and most stores have no
   row-level security to catch it).

8. **Verify** the fix actually resolves the bug (trace the real code path, or `/run` to launch
   the app and observe it). Don't claim fixed without evidence.

   Narrow question here: *did the bug go away*. The authoritative pass at the final fingerprint, and the
   only dispatch of a UI verifier agent, is `/ship` step 4 — see `/fix` Step 6 for the division. If the
   bug was visual, look at it in the browser yourself now; that also leaves it logged in, which the
   verifier at step 9 inherits and cannot do for itself.

   **A clean check is not yet evidence.** A positive result carries its own content; a *negative*
   one — "no match", "0 results", "OK", "nothing found" — that lets you proceed does not, until a
   second independent mechanism agrees. Sweep with `rg`, not `grep -R`; confirm a wrapper tool's
   "not found" against the underlying tool; on a connection failure run `nc -z <host> <port>`
   before theorising. Five times in one session a tool reported clean and the clean was wrong.

9. **`/gate`, then `/ship` — both, always.** `/gate` is cheap and read-only and tells you which
   repos the change reaches. **Then `/ship`, unconditionally** — not only when the gate reports
   cross-repo impact, and not skipped because the incident is urgent.

   This is production, written fast, by a session that also verified its own work — the exact
   single-control setup where **75.8%** of self-reported success claims were false, and where an
   LLM judge asked to catch that reaches only AUROC 0.65. What closed the gap was an independent
   party outside the agent's own control: **48% → 3%**. `/ship`'s reviewer is that party. Being
   in a hurry is the reason it is needed, not a reason to skip it.

   If `/ship` is all green it commits and pushes the hotfix branch (house rule #7 — a
   `hotfix/*` branch is not a deploy branch, so pushing it deploys nothing). If verification
   failed, do not commit; fix first.

10. **Remind the user of the remaining manual steps:** they create the PR against the resolved
    base ref by hand and merge it (= deploy), then **sync the fix back to the repo's normal
    feature base** (merge/cherry-pick) so the next release doesn't lose it. Once that sync is
    done, tear down each worktree with `wt rm $ARGUMENTS` (run in each affected repo) — it
    refuses if the branch has no merge trace, and detects a squash-merge by content, so it
    usually just works.

## Output
```
Affected repos: ...
Base ref confirmed: <ref, e.g. origin/release12 or staging — the code actually running>
Root cause confirmed: <cause, with the real error output that showed it>
Response chosen: revert | disable | fix forward  — <why, and what was rejected>

--- forward fix only ---
Hotfix branch: <username>/hotfix/<desc> — worktrees: <repo>/.worktrees/<desc> (port ...)
Scout @ <base ref>: <blast radius> · uncovered by tests: <...> · confidence: <...>
Fix verified: ✅/❌ (how — and at which fingerprint)
/gate: <N repos impacted, or clean>
Pushed: <username>/hotfix/<desc> → origin.

<-- cat ${TMPDIR:-/tmp}/ship-board-<fp10>.md  — the gate's board, read out of the
    file it wrote. Every line, including ❌ ones, skipped ones, and Gate log.
    Not "/ship: ✅ all green". -->
---

Next (yours): create PR → <base ref> by hand → merge (= deploy) → sync fix back to the normal
feature base.
```

**The board is `cat`ed from `/ship`'s file, not retyped.** Every check that carries weight in a hotfix
runs inside that gate, so `"/ship: ✅ all green"` hides all of it — and this is the pipeline where that
matters most: code written under time pressure, landing in production, verified by the same session that
wrote it. The per-check fingerprints are the only part you can confirm without trusting my summary, and
collapsing the board throws exactly them away. Reading it from a file also means skipping this step
leaves a missing tool call rather than a sentence that reads fine either way.

(`at HEAD` was also wrong: `/ship` commits last, so the tree is dirty during every check and `HEAD` is
not what was tested — the fingerprint is.)
