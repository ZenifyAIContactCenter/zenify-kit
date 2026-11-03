---
name: hotfix
description: Handle a live production bug: confirm the hotfix base from release tags, choose revert, disable or fix forward, branch the fix from that base. Asks only when tags cannot confirm the base, the last deploy may be the cause, or the fix is big. Run from `/fix` Step 2b or when the user asks.
argument-hint: "[short-kebab-desc]"
allowed-tools: Bash(git *) Bash(wt *) Bash(zenify *) Read Grep Bash(rg *) Bash(cat *) Bash(head *) Bash(sed *) Bash(cut *) Bash(grep *) Bash(sort *) Bash(wc *) Bash(printf *) Write Agent
---

Hotfix: **<slug>**

`<slug>` is the first word after the skill name; any lines after it are a `/fix` handoff (step 3).

Tool names for each action: `znf:_shared/harness-tools` (harness mapping table).

## How much diagnosis

Narrow or Wide, as in `/fix` § How much diagnosis; Wide → `EnterPlanMode` first. It scales step 3,
never skips it. State the path and why in one line (rationale: references).

## Steps

1. **Affected repo(s):** route per the workspace's CLAUDE.md; unclear → locate it, never guess.

2. **Fetch first, then resolve and confirm the hotfix base ref** for each affected repo:
   ```bash
   git -C <repo> fetch origin --tags
   REF=$(zenify hotfix baseref <repo>)
   CUR=$(git -C <repo> tag --merged "$REF" --list 'v[0-9]*' --sort=-v:refname | head -1)
   LATEST=$(git -C <repo> tag --list 'v[0-9]*' --sort=-v:refname | head -1)
   BASE=$(cd <repo> && zenify wt base)
   printf 'REF=%s CUR=%s LATEST=%s BASE=%s\n' "$REF" "$CUR" "$LATEST" "$BASE"
   ```
   `CUR` empty, `CUR ≠ LATEST`, or `REF` = `BASE` (no own hotfix base: tags cannot tell an
   integration branch from production) → ask the user to confirm `$REF` and wait. Otherwise print
   `Base ref confirmed: $REF (contains $CUR = latest tag)` and go on. Variables do not survive
   between Bash calls: later steps reuse the printed values. An error means a stale
   `worktree.json`: fix it, never guess a branch name. Steps 6 and 9 need this ref too.

3. **Diagnose — before touching anything.** If handed off from `/fix`, its Step 0–2 evidence and
   confirmed cause carry over — do not re-diagnose; continue from step 4. Otherwise run **`/fix`
   steps 0-2**, grounded in real error output (`znf:_shared/evidence-sources`). If you cannot
   confirm the cause, say so — **do not guess-fix production.** On production run read-only rungs directly, redact output
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
   REF=<ref>   # step 2's printed ref
   PREV=$(git -C <repo> tag --merged "$REF" --list 'v[0-9]*' --sort=-v:refname | sed -n 2p)
   N=$(git -C <repo> show "$REF:<file>" | grep -nF -f <needle> | cut -d: -f1)
   C=$(case "$N" in (''|*[!0-9]*) ;; (*) git -C <repo> blame -L"$N,$N" --porcelain "$REF" -- <file> | head -1 | cut -d' ' -f1 ;; esac)
   printf 'PREV=%s N=%s C=%s\n' "$PREV" "$N" "$C"
   git -C <repo> merge-base --is-ancestor "$C" "$PREV"   # success = bug predates PREV
   ```
   `<needle>`: a file holding the faulty line, trimmed, written with Write — never paste source
   into shell quotes. Success → fix forward without asking; record
   `Response chosen: fix forward — bug predates <PREV> (introduced in <C>)`. Ask (the three with a
   recommendation, then **wait**) when there is no `PREV`, you cannot pin the faulty line (data
   or config bug, `N` not exactly one line), `C` is empty, or the ancestor check fails. Limits of
   this check: references.

   **If the answer is revert or disable: stop here.** Do it, verify the incident is over, and
   skip to step 10 — no worktree, no scout, no fix. Plan the real fix calmly afterwards on the
   repo's normal feature base.

5. **Create the hotfix worktree** — forward fix only, always (house rule #8), one per affected
   repo, same slug:
   ```bash
   cd <repo> && wt new <slug> --type hotfix --base <ref>
   ```
   `<ref>` = step 2's printed ref, never empty; `--type hotfix` is **exempt** from the one-task
   guard (references). `<worktree>` below is its path; work there, not in the main checkout.

6. **`Skill(scout)` the RESOLVED ref, not the default feature base** — as a call, briefed as
   `/fix` Step 3 does.

   **Getting the ref wrong fails silently.** State which ref you scouted.

7. **Fix.** Red command and regression test first, then the smallest diff — `/fix` Step 5;
   refactor nothing else.

   **`Skill(ground)` anything unverified this session**, including the values a field holds and
   the filter every query must carry.

8. **Verify**: re-run the red command and clear `[DEBUG-` tags — `/fix` Step 6 (or `/run` the app).

   Only *did the bug go away*: the look and the only `znf:ui-verifier` dispatch are `/ship`
   step 4. A clean (negative) check needs a second mechanism (references § Clean checks).

9. **`/gate`, then `/ship` — both, always.** `/gate` is cheap, read-only, and tells you which
   repos the change reaches. Before `/ship` (nothing is committed yet), count the changed files:
   ```bash
   B=$(git -C <worktree> merge-base <ref> HEAD) && { git -C <worktree> diff --name-only "$B"; git -C <worktree> ls-files --others --exclude-standard; } | sort -u | wc -l
   ```
   `<ref>` = step 2's printed ref. No number printed, over 3, or `/gate` reports a shared collection, inter-service HTTP, pub/sub or queue → ask,
   recommending revert, and wait; never run `/cook` on a hotfix base. Tell `/ship` it ships a
   hotfix: after each fix wave, before committing, re-run this count and `/gate`, same asks. **Then `/ship`,
   unconditionally** — urgency is no exemption (references).

   All green → `/ship` commits, pushes the hotfix branch and opens the PR into `$REF`. Verification
   failed → no commit; fix first.

10. **After `/ship`:** the user merges the PR (= deploy), then syncs `$REF` → the repo's `baseRef`
    (skip when they are the same ref) so the next release keeps the fix. Once production runs it
    (`git -C <repo> fetch origin --tags` then `git -C <repo> tag --contains <merge-sha> --list 'v[0-9]*'`
    print a tag, or the build check in `references/post-deploy-check.md` finds it), check the area the fix touched — the checks that fit, not all of them:
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

<-- cat ${TMPDIR:-/tmp}/ship-board-<fp10>.md — every line, never retyped -->
---

PR opened: <url> → <base ref>. Next (yours): merge (= deploy) → sync <base ref> → baseRef.
```

## References

- `references/why-these-rules.md` — why diagnosis precedes the worktree, how much diagnosis, why tags decide steps 2 and 4, why clean checks need a second mechanism, and the evidence behind the unconditional `/ship`.
- `references/post-deploy-check.md` — which numbers to compare before and after the deploy, per changed area (backend, web, queue).
