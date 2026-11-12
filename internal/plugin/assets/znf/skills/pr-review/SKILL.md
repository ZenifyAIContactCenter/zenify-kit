---
name: pr-review
description: After a PR is open, waits for the project's declared PR review check on the head commit, fixes or declines what blocks it, pushes and waits again, up to 3 rounds; never merges. /ship invokes it after `gh pr create`; use it directly on a PR number or URL to handle the bot review.
allowed-tools: Bash(git *) Bash(gh pr view*) Bash(gh pr comment*) Bash(gh api *) Read Grep
---

Tool names for each action: `znf:_shared/harness-tools`.

Runs once per open PR. `REPO` = `<owner/repo>`, `PR` = its number.
- **From ship**, right after `gh pr create`: both are ship's.
- **By hand**, with a PR number or URL: `gh pr view <n|url> --json number,url,headRefName,headRefOid`. The current branch must be `headRefName` with `HEAD` = `headRefOid`; otherwise say so and stop. Read `znf:ship` step 2 and its `test-scope.md` before the first fix: step 6 runs them.

**Watch applies** when ship has `--watch`, or a `track` run treats ship as `--watch`, or the user passed `--watch` here, and the args carry no `CI round` line. Every stop below ends only this loop: when watch applies, still run `Skill(znf:ci-watch)` (forward a `track id:` line) before reporting.

## 1. Find the declaration
Walk up from the worktree to `/`, reading each `CLAUDE.md`; the first line starting `PR review check:` wins. It names the check, where its findings live, the decline syntax and the re-run command, and it may name a tool for each of: wait, decline, re-run. Follow those exactly; never assume a bot, check name, tool name or comment format of your own. The check must be a check run (`CheckRun`); a legacy commit status is not polled — report that and stop.

If no such line exists, skip the loop: run `Skill(znf:ci-watch)` when watch applies; otherwise report the PR URL and stop.

## 2. Pick the exchange path
- **Tool path**: the line names all three tools and this session has them ("Call a tool the project declares"). Wait, decline and re-run through the tools and post nothing on the PR: no decline line, no re-run command, no thread reply. The bot posts its own report.
- **Comment path**: a tool is missing or a call fails → for the rest of this run use the declared comment syntax, and after each push handle the review threads per `references/comment-threads.md`.

## 3. Wait for the verdict
`SHA` = the head you pushed, `PREV` = the head before it (empty on the first wait), `SINCE` = the `startedAt` of the run being replaced, from 5.5 (empty after a push).

Tool path: first call the wait tool for `SHA` (same `SHA` as the last wait → pass the value it gave to tell runs apart). It returns when the bot is done; its findings are step 5's list. Not done → call again, for 30 minutes in all, then stop and report as exit 1 below. It reports a failed run → the re-run tool once, then wait again; failed twice → stop and report.

Then, on both paths, run in the background (`run_in_background`), like ci-watch's loops; on the tool path it exits at the first poll:

```bash
REPO=<owner/repo>; PR=<n>; SHA=<sha>; PREV=<sha or empty>; SINCE=<time or empty>; deadline=$((SECONDS+1800))
while :; do
  if row=$(gh pr view "$PR" -R "$REPO" --json headRefOid,statusCheckRollup --jq '[.headRefOid] + ([.statusCheckRollup[] | select(.__typename=="CheckRun" and .name=="<check>")] | sort_by(.startedAt) | last | [.status, .conclusion, .startedAt]) | map(if . == null or . == "" then "-" else . end) | @tsv'); then
    IFS=$'\t' read -r head st concl started <<<"$row"
    if [ "$head" = "$SHA" ]; then
      [ "$st" = COMPLETED ] && [[ "$started" > "$SINCE" ]] && { echo "$concl $started"; exit 0; }
    elif [ "$head" != "$PREV" ]; then echo "head moved to $head"; exit 2; fi
  fi
  [ "$SECONDS" -lt "$deadline" ] || { echo "no verdict after 30 minutes: $st $concl"; exit 1; }
  sleep 30
done
```

Wait for only that check, never the PR's other checks. Exit 1 → stop and report the PR URL and the check's state. Exit 2 (someone else pushed) → stop and report.

## 4. Read the verdict
Green or red is that check's `conclusion`, set by the bot. Never infer it from the findings list or a tool result.
- `SUCCESS` or `NEUTRAL` → green: go to 7.
- `FAILURE` → red: go to 5.
- Anything else (`CANCELLED`, `SKIPPED`, `TIMED_OUT`, `ACTION_REQUIRED`, `STALE`) → stop and report the PR URL and the conclusion; it costs no round.

## 5. Red: round N of 3
One round is one push, or one decline or re-run with nothing pushed. Declines sent with a push cost no round of their own.
1. Read the findings: the wait tool's result, or on the comment path the declared source. Read the code each one points to before changing anything.
2. Fix every blocking finding (CRITICAL or HIGH, not declined) plus every CI check already red at this moment, in ONE commit.
3. MEDIUM: fold it into the same commit, or mark it to decline, with a reason. Never push a round that fixes only MEDIUM findings: decline them with reasons and take the 5.5 path instead.
4. A wrong finding: mark it to decline, with the evidence as the reason; do not change code for it. Marked declines are sent once, in 5.5 or in 6.4: through the decline tool, or on the comment path with the declared decline syntax.
5. Nothing left to push after declines → comment path: post the decline lines, answer their threads per the reference, set `SINCE` to the `startedAt` the last wait printed (GitHub's clock, not yours), post the declared re-run command. Tool path: call the decline tool. Its result carries a new verdict → leave `SINCE` empty (the bot may have updated the same run); otherwise set `SINCE` the same way and call the re-run tool. Go back to 3 with the same `SHA`.

## 6. Before each push
1. `git fetch`. If the remote branch moved, rebase the unpushed fix commit onto it; never force-push. A rebase conflict → stop and report. The head the bot reviewed stays an ancestor, so `BASE` still resolves.
2. Re-run `znf:ship` step 2 (lint, build/typecheck) on the fixed tree, then the tests related to the fix, scoped as in ship's `test-scope.md`. A lint or build result from before the fix proves nothing about it.
3. `Skill(znf:review)` with `BASE` = the head the bot last reviewed; pass `STATIC_OK=1` only when 2's lint and build ran on this exact tree. Fix what the review finds in the same commit; that fix voids 2 and 3, so re-run 2, then re-review only the fix (`git diff <head the last review saw>..HEAD`, its findings verbatim, each ADDRESSED or NOT ADDRESSED). These passes cost no round. A finding still NOT ADDRESSED after the second re-review → do not push; leave the commit local, stop and report the finding, what was tried and whether it is load-bearing.
4. Send the declines marked in 5 (tool path: the decline tool, before the push). Push, set `PREV` to the remote head you fetched in 1 (the PR head before your push), `SHA` to the new head, `SINCE` empty. Comment path: the threads, per the reference. Go back to 3.

## 7. Stop
- The check is green → wait for the PR's other checks via ci-watch step 4 (when watch applies, run the full `Skill(znf:ci-watch)` instead), then report, naming each open non-blocking finding; push nothing more for them.
- Still red after 3 rounds → stop and report each open finding and what was tried.
- Never merge, and print a merge command only when the check is green on the PR's current head.

## References
- `references/comment-threads.md` — reply to and resolve review threads on the comment path.
