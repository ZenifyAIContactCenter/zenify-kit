---
name: hotfix
description: Handle a live production bug in a polyrepo workspace — diagnose first, then decide the response with the user (revert, disable, or fix forward), and only for a forward fix create an isolated worktree branched from the repo's configured hotfix base ref (never the default feature base). Scouts what depends on the code, verifies, gates and ships. Commits + pushes the hotfix branch once verified and opens the PR, but never merges. User-invoked only (whether something is a hotfix is the user's urgency call). If a bug looks live-critical, you may SUGGEST running /hotfix, but don't run it automatically.
disable-model-invocation: true
argument-hint: "[short-kebab-desc]"
allowed-tools: Bash(git *) Bash(wt *) Bash(zenify *) Read Grep Bash(rg *) Bash(cat *) Agent
---

Hotfix: **$ARGUMENTS**

Tool names for each action: `znf:_shared/harness-tools` (harness mapping table).

## How much diagnosis

This scales step 3; it never skips it. Confirming the root cause against real error output is
required on both paths.

- **Narrow** — one clear error, one obvious cause → one hypothesis, checked against the real
  error output, then confirm.
- **Wide** — cause unclear, multi-service, or hard to reproduce → `EnterPlanMode` (Opus) to run
  parallel hypotheses first, `ExitPlanMode` before deciding the response.

State which path and why (one line). If the narrow path's single hypothesis does not survive
confirmation, you were on the wide path all along.

## Steps

1. **Affected repo(s):** decide which repo(s) the live bug is in (route per the workspace's own
   CLAUDE.md). If unclear, locate it first — don't guess.

2. **Fetch first, then resolve the hotfix base ref** for each affected repo, then **confirm with
   the user** (deploy-critical — the base ref differs per repo, and is never guessed from a
   pattern here):
   ```bash
   git -C <repo> fetch origin              # BEFORE resolving the base ref — see below
   REF=$(zenify hotfix baseref <repo>)
   echo "$REF"                              # confirm with the user that this is what is live
   ```
   **Fetch BEFORE resolving the release ref** — a stale local ref branches from the wrong release
   entirely, and the tool's own fetch is too late. See references. You need this ref even if you
   never branch from it: steps 6 and 9 depend on knowing which code is actually running.

3. **Diagnose — before touching anything.** Run **`/fix` steps 0-2**: ground in the real error
   output first (logs, stacktrace, `docker logs <container>`), then hypotheses, then confirm the
   root cause. If you cannot confirm it, say so — **do not guess-fix production.** This comes
   before the worktree on purpose (see references).

4. **Decide the response — the user's call, not yours.** With the cause confirmed, there are
   three responses and forward-fixing is only one:

   ```
   ▸ REVERT the deploy or commit that introduced it   — usually fastest and safest
   ▸ DISABLE the path (feature flag / kill switch)    — if that path has one
   ▸ FIX FORWARD                                      — only when neither above applies,
                                                         or the bug predates the last deploy
   ```

   Present the three with a recommendation and **wait**.

   **If the answer is revert or disable: stop here.** Do it, verify the incident is over, and
   skip to step 10 — no worktree, no scout, no fix. Plan the real fix calmly afterwards on the
   repo's normal feature base.

   **If the forward fix needs feature-sized work** (several files, a contract change, a design
   decision), that is the signal to **revert instead**. Do not run `/cook` on a hotfix base ref.

5. **Create the hotfix worktree** — forward fix only, and always a worktree (house rule #8). Use
   the `$REF` you resolved **after fetching** in step 2; same slug across repos so the branch name
   matches:
   ```bash
   cd <repo> && wt new $ARGUMENTS --type hotfix --base "$REF"
   ```
   One worktree per affected repo. Do not re-derive `$REF` here — the value from step 2 is the
   one the user confirmed. `--type hotfix` is deliberately **exempt** from the
   one-task-per-repo-per-session guard. Work inside the worktree path, not the main checkout.

6. **`Skill(scout)` — and scout the RESOLVED ref, not the default feature base.** Invoke it as a
   call, not as an intention: `Skill(scout)` and the `Agent(scout)` it dispatches each leave a
   line carrying their name, so a skipped step is visible instead of having to be taken on trust.
   Find consumers, the tests that cover it, anything written in the same operation, and why those
   lines exist (`git blame -L`, `git log -S`).

   **The ref matters and getting it wrong fails silently.** Scout the `$REF` resolved in step 2,
   not the repo's normal feature base. State which ref you scouted.

7. **Fix.** Read the real code first, smallest diff, and do not refactor anything you are not
   fixing.

   **Call `Skill(ground)` on anything unverified this session** — not just "does the field exist"
   but **which values it holds** (distinct-values query) and **which filter every query must
   carry** (tenant/scope).

8. **Verify** the fix actually resolves the bug (trace the real code path, or `/run` to launch
   the app and observe it). Don't claim fixed without evidence.

   Narrow question here: *did the bug go away*. The authoritative pass, and the only dispatch of
   a UI verifier agent, is `/ship` step 4 — see `/fix` Step 6. If the bug was visual, look at it
   in the browser yourself now; that leaves it logged in for the verifier at step 9.

   **A clean check is not yet evidence.** A negative result that lets you proceed needs a second
   independent mechanism to agree. Sweep with `rg`, not `grep -R`; confirm a wrapper tool's "not
   found" against the underlying tool; on a connection failure run `nc -z <host> <port>` first.

9. **`/gate`, then `/ship` — both, always.** `/gate` is cheap, read-only, and tells you which
   repos the change reaches. **Then `/ship`, unconditionally** — not only on cross-repo impact,
   and not skipped because the incident is urgent (see references).

   If `/ship` is all green it commits and pushes the hotfix branch (house rule #7 — a
   `hotfix/*` branch is not a deploy branch, so pushing it deploys nothing). If verification
   failed, do not commit; fix first.

10. **Remind the user of the remaining manual steps:** they create the PR against the resolved
    base ref by hand and merge it (= deploy), then **sync the fix back to the repo's normal
    feature base** (merge/cherry-pick) so the next release doesn't lose it. Once synced, tear
    down each worktree with `wt rm $ARGUMENTS` (in each affected repo).

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

PR opened: <url> → <base ref>. Next (yours): merge (= deploy) → sync fix back to the normal
feature base.
```

**The board is `cat`ed from `/ship`'s file, not retyped.** Collapsing it to a verdict throws away
the per-check fingerprints — the only part a reader can confirm without trusting me.

## References

- `references/why-these-rules.md` — why diagnosis precedes the worktree, why the base ref is fetched before it is resolved, why the response is the user's call, and the evidence behind the unconditional `/ship`.
