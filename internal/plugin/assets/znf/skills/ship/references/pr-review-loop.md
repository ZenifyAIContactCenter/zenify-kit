# Ship step 7: the PR review loop

Runs once per opened PR, right after `gh pr create`. `REPO` = `<owner/repo>`, `PR` = its number.

**Watch applies** when ship has `--watch`, or a `track` run treats ship as `--watch`, and the args carry no `CI round` line. Every stop below ends only this loop: when watch applies, still run `Skill(znf:ci-watch)` before reporting.

## 1. Find the declaration
Walk up from the worktree to `/`, reading each `CLAUDE.md`; the first line starting `PR review check:` wins. It names the check, where its findings live, the decline syntax and the re-run command. Follow those exactly; never assume a bot, check name or comment format of your own. The check must be a check run (`CheckRun`); a legacy commit status is not polled — report that and stop.

If no such line exists, skip the loop: run `Skill(znf:ci-watch)` when watch applies; otherwise report the PR URL and stop.

## 2. Wait for the verdict
`SHA` = the head you pushed, `PREV` = the head before it (empty on the first wait), `SINCE` = the `startedAt` of the run being replaced, from 4.5 (empty after a push). Run in the background (`run_in_background`), like ci-watch's loops:

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

## 3. Read the verdict
Green or red is that check's `conclusion`, set by the bot. Never infer it from the findings list.
- `SUCCESS` or `NEUTRAL` → green: go to 6.
- `FAILURE` → red: go to 4.
- Anything else (`CANCELLED`, `SKIPPED`, `TIMED_OUT`, `ACTION_REQUIRED`, `STALE`) → stop and report the PR URL and the conclusion; it costs no round.

## 4. Red: round N of 3
A push or a re-run each counts as one round.
1. Read the findings from the declared source.
2. Fix every blocking finding (CRITICAL or HIGH, not declined) plus every CI check already red at this moment, in ONE commit.
3. MEDIUM: fold it into the same commit, or reply with the declared decline syntax and a reason. Never push a round that fixes only MEDIUM findings: decline them with reasons and take the 4.5 path instead.
4. A wrong finding: decline it with the declared decline syntax and a reason; do not change code for it.
5. Nothing left to push after declines → post the decline lines, set `SINCE` to the `startedAt` the last wait printed (GitHub's clock, not yours), post the declared re-run command, go back to 2 with the same `SHA`.

## 5. Before each push
1. `git fetch`. If the remote branch moved, rebase the unpushed fix commit onto it; never force-push. A rebase conflict → stop and report. The head the bot reviewed stays an ancestor, so `BASE` still resolves.
2. Run the tests related to the fix, scoped as in `test-scope.md`.
3. `Skill(znf:review)` with `BASE` = the head the bot last reviewed. Fix what the review finds in the same commit; that fix voids 2 and 3, so re-run 2, then re-review only the fix (`git diff <head the last review saw>..HEAD`, its findings verbatim, each ADDRESSED or NOT ADDRESSED). These passes cost no round. A finding still NOT ADDRESSED after the second re-review → do not push; leave the commit local, stop and report the finding, what was tried and whether it is load-bearing.
4. Push, set `PREV` to the remote head you fetched in 1 (the PR head before your push), `SHA` to the new head, `SINCE` empty, go back to 2.

## 6. Stop
- The check is green → wait for the PR's other checks via ci-watch step 4 (when watch applies, run the full `Skill(znf:ci-watch)` instead), then report.
- Still red after 3 rounds → stop and report each open finding and what was tried. Never merge.
