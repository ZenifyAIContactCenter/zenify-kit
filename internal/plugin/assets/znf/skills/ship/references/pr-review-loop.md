# Ship step 7: the PR review loop

Runs once per opened PR, right after `gh pr create`. `REPO` = `<owner/repo>`, `PR` = its number.

## 1. Find the declaration
Walk up from the worktree to `/`, reading each `CLAUDE.md`; the first line starting `PR review check:` wins. It names the check, where its findings live, the decline syntax and the re-run command. Follow those exactly; never assume a bot, check name or comment format of your own.

If no such line exists, report the PR URL and stop (the step 7 behaviour without this loop).

## 2. Wait for the verdict
`SHA` = the head you pushed, `PREV` = the head before it (empty on the first wait), `SINCE` = the re-run time from 4.5 (empty after a push). Run in the background (`run_in_background`), like ci-watch's loops:

```bash
REPO=<owner/repo>; PR=<n>; SHA=<sha>; PREV=<sha or empty>; SINCE=<time or empty>; deadline=$((SECONDS+1800))
while :; do
  row=$(gh pr view "$PR" -R "$REPO" --json headRefOid,statusCheckRollup --jq '[.headRefOid] + ([.statusCheckRollup[] | select(.__typename=="CheckRun" and .name=="<check>")] | sort_by(.startedAt) | last | [.status, .conclusion, .startedAt]) | @tsv') || { sleep 30; continue; }
  IFS=$'\t' read -r head st concl started <<<"$row"
  if [ "$head" = "$SHA" ]; then
    [ "$st" = COMPLETED ] && [[ "$started" > "$SINCE" ]] && { echo "$concl"; exit 0; }
  elif [ "$head" != "$PREV" ]; then echo "head moved to $head"; exit 2; fi
  [ "$SECONDS" -lt "$deadline" ] || { echo "no verdict after 30 minutes: $st $concl"; exit 1; }
  sleep 30
done
```

Wait for only that check, never the PR's other checks. Exit 1 → stop and report the PR URL and the check's state. Exit 2 (someone else pushed) → stop and report.

## 3. Read the verdict
Green or red is that check's `conclusion`, set by the bot. Never infer it from the findings list.

## 4. Red: round N of 3
A push or a re-run each counts as one round.
1. Read the findings from the declared source.
2. Fix every blocking finding (CRITICAL or HIGH, not declined) plus every CI check already red at this moment, in ONE commit.
3. MEDIUM: fold it into the same commit, or reply with the declared decline syntax and a reason. Never push a round that fixes only MEDIUM findings: decline them with reasons and take the 4.5 path instead.
4. A wrong finding: decline it with the declared decline syntax and a reason; do not change code for it.
5. Nothing left to push after declines → post the decline lines, set `SINCE=$(date -u +%FT%TZ)`, post the declared re-run command, go back to 2 with the same `SHA`.

## 5. Before each push
1. `git fetch`. If the remote branch moved, rebase the unpushed fix commit onto it; never force-push. A rebase conflict → stop and report. The head the bot reviewed stays an ancestor, so `BASE` still resolves.
2. Run the tests related to the fix, scoped as in `test-scope.md`.
3. `Skill(znf:review)` with `BASE` = the head the bot last reviewed; fix what it finds in the same commit.
4. Push, set `PREV` to the old head, `SHA` to the new head, `SINCE` empty, go back to 2.

## 6. Stop
- The check is green → wait for the PR's other checks via ci-watch step 4 (with `--watch`, run the full `Skill(znf:ci-watch)` instead), then report.
- Still red after 3 rounds → stop and report each open finding and what was tried. Never merge.
