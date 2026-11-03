---
name: hotfix
description: Handle a live production bug: confirm the hotfix base from release tags, decide revert, disable or fix forward (asks only when the last deploy may be the cause), then branch the fix from that base. `/fix` hands production bugs here.
argument-hint: "[short-kebab-desc]"
allowed-tools: Bash(git *) Bash(wt *) Bash(zenify *) Read Grep Bash(rg *) Bash(cat *) Agent
---

Hotfix: **<slug>**

`<slug>` is the text after the skill name.

Tool names for each action: `znf:_shared/harness-tools` (harness mapping table).

## How much diagnosis

Narrow or Wide, as in `/fix` § How much diagnosis; Wide → `EnterPlanMode` first. It scales step 3,
never skips it. State the path and why in one line (rationale: references).

## Steps

1. **Affected repo(s):** decide which repo(s) the live bug is in (route per the workspace's own
   CLAUDE.md). If unclear, locate it first — don't guess.

2. **Fetch first, then resolve and confirm the hotfix base ref** for each affected repo:
   ```bash
   git -C <repo> fetch origin --tags       # BEFORE resolving the base ref — see below
   REF=$(zenify hotfix baseref <repo>)
   CUR=$(git -C <repo> tag --merged "$REF" --list 'v[0-9]*' --sort=-v:refname | head -1)
   LATEST=$(git -C <repo> tag --list 'v[0-9]*' --sort=-v:refname | head -1)
   ```
   `CUR` empty or `CUR ≠ LATEST` → ask the user to confirm `$REF` and wait. Otherwise print
   `Base ref confirmed: $REF (contains $CUR = latest tag)` and go on. **Fetch BEFORE resolving the
   base ref** — the tool's own fetch is too late. An error means a stale `worktree.json`: fix it,
   never guess a branch name. See references. You need this ref even without branching from it:
   steps 6 and 9 depend on it.

3. **Diagnose — before touching anything.** If handed off from `/fix`, its Step 0–2 evidence and
   confirmed cause carry over — do not re-diagnose; continue from step 4. Otherwise run **`/fix`
   steps 0-2**: ground in the real error output first (logs, traces, metrics:
   `znf:_shared/evidence-sources`), then hypotheses, then confirm the root cause. If you cannot
   confirm it, say so — **do not guess-fix production.** This comes before the worktree on
   purpose (see references). On production run read-only rungs directly, redact output
   (`<REDACTED>`); a side-effect rung goes via `zenify data run` (dry-run; user's "ok" before
   `--prd --write`); with no read-only loop, confirm the cause from logs — the no-loop
   stop and temporary logging do not apply.

4. **Decide the response.** With the cause confirmed, there are three responses and
   forward-fixing is only one:

   ```
   ▸ REVERT the deploy or commit that introduced it   — usually fastest and safest
   ▸ DISABLE the path (feature flag / kill switch)    — if that path has one
   ▸ FIX FORWARD                                      — only when neither above applies,
                                                         or the bug predates the last deploy
   ```

   Check whether the bug predates the previous release tag:
   ```bash
   PREV=$(git -C <repo> tag --merged "$REF" --list 'v[0-9]*' --sort=-v:refname | sed -n 2p)
   C=$(git -C <repo> log -S'<faulty line>' --format=%H --reverse "$REF" -- <file> | head -1)
   git -C <repo> merge-base --is-ancestor "$C" "$PREV"   # success = bug predates PREV
   ```
   Success → fix forward without asking; record
   `Response chosen: fix forward — bug predates <PREV> (introduced in <C>)`. Ask (the three with a
   recommendation, then **wait**) when there is no `PREV`, you cannot pin the faulty line (data
   or config bug), `C` is empty, or the ancestor check fails. A renamed file makes `C` the rename
   commit — later than the truth — so it only adds a question (references).

   **If the answer is revert or disable: stop here.** Do it, verify the incident is over, and
   skip to step 10 — no worktree, no scout, no fix. Plan the real fix calmly afterwards on the
   repo's normal feature base.

5. **Create the hotfix worktree** — forward fix only, and always a worktree (house rule #8). Use
   the `$REF` you resolved **after fetching** in step 2; same slug across repos so the branch name
   matches:
   ```bash
   cd <repo> && wt new <slug> --type hotfix --base "$REF"
   ```
   One worktree per affected repo. Do not re-derive `$REF` — use step 2's value. `--type hotfix`
   is deliberately **exempt** from the one-task-per-repo-per-session guard. Work inside the
   worktree path, not the main checkout.

6. **`Skill(scout)` — and scout the RESOLVED ref, not the default feature base.** Invoke it as a
   call — `Skill(scout)` leaves a named line.
   Find consumers, the tests that cover it, anything written in the same operation, and why those
   lines exist (`git blame -L`, `git log -S`).

   **Getting the ref wrong fails silently.** State which ref you scouted.

7. **Fix.** Red command and regression test first, then the smallest diff — `/fix` Step 5;
   refactor nothing else.

   **Call `Skill(ground)` on anything unverified this session** — not just "does the field exist"
   but **which values it holds** (distinct-values query) and **which filter every query must
   carry** (tenant/scope).

8. **Verify**: re-run the red command and clear `[DEBUG-` tags — `/fix` Step 6. Don't claim
   fixed without evidence (or `/run` the app and observe it).

   Narrow question here: *did the bug go away*. The authoritative pass, and the only dispatch of
   `znf:ui-verifier`, is `/ship` step 4. If the bug was visual, the look is checked there, not by
   you. A clean (negative) check needs a second mechanism to agree (references § Clean checks).

9. **`/gate`, then `/ship` — both, always.** `/gate` is cheap, read-only, and tells you which
   repos the change reaches. Before `/ship`: if `git diff --name-only "$REF"...HEAD | wc -l` is
   > 3, or `/gate` reports a shared collection, inter-service HTTP, pub/sub or queue → ask,
   recommending revert, and wait; never run `/cook` on a hotfix base. **Then `/ship`,
   unconditionally** — not only on cross-repo impact, and not skipped because the incident is
   urgent (see references).

   If `/ship` is all green it commits, pushes the hotfix branch and opens the PR into `$REF`
   (house rule #7 — a `hotfix/*` branch is not a deploy branch). If verification failed, do not
   commit; fix first.

10. **After `/ship`:** the user merges the PR (= deploy), then syncs `main → staging` so the next
    release keeps the fix. Once `git -C <repo> tag --contains <merge-sha> --list 'v[0-9]*'` prints
    a tag, check the area the fix touched — the checks that fit, not all of them:
    `references/post-deploy-check.md`. Add the numbers to the hotfix report. Tear down with
    `wt sweep`.

## Output
```
Affected repos: ...
Base ref confirmed: <ref> (contains <CUR> = latest tag) — or: confirmed by the user
Root cause confirmed: <cause, with the real error output that showed it>
Response chosen: revert | disable | fix forward  — <why; C and PREV when decided by tags>

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

PR opened: <url> → <base ref>. Next (yours): merge (= deploy) → sync main → staging.
```

## References

- `references/why-these-rules.md` — why diagnosis precedes the worktree, how much diagnosis, why tags decide steps 2 and 4, why clean checks need a second mechanism, and the evidence behind the unconditional `/ship`.
- `references/post-deploy-check.md` — which numbers to compare before and after the deploy, per changed area (backend, web, queue).
