# Ship step 7: the PR review loop

Runs once per opened PR, right after `gh pr create`. `REPO` = `<owner/repo>`, `PR` = its number.

## 1. Find the declaration
Walk up from the worktree to `/`, reading each `CLAUDE.md`; the first line starting `PR review check:` wins. It names the check, where its findings live, the decline syntax and the re-run command. Follow those exactly; never assume a bot, check name or comment format of your own.

If no such line exists, report the PR URL and stop (the step 7 behaviour without this loop).

## 2. Wait for the verdict
`SHA` = the head you just pushed. Poll in the background, like ci-watch's loops:

```bash
gh pr view "$PR" -R "$REPO" --json headRefOid,statusCheckRollup \
  --jq '[.headRefOid, (.statusCheckRollup[] | select(.__typename=="CheckRun" and .name=="<check>") | .status, .conclusion)] | @tsv'
```

Act only when `headRefOid` equals `SHA` and the declared check's `status` is `COMPLETED`. Wait for only that check, never the PR's other checks. No `COMPLETED` run of it on `SHA` within 30 minutes → stop and report the PR URL and the check's state.

## 3. Read the verdict
Green or red is that check's `conclusion`, set by the bot. Never infer it from the findings list.

## 4. Red: one round
1. Read the findings from the declared source.
2. Fix every blocking finding (CRITICAL or HIGH, not declined) plus every CI check already red at this moment, in ONE commit.
3. MEDIUM: fold it into the same commit, or reply with the declared decline syntax and a reason. Never push a round that fixes only MEDIUM findings.
4. A wrong finding: decline it with the declared decline syntax and a reason; do not change code for it.
5. Nothing left to push after declines → post the decline lines, then the declared re-run command. That counts as a round; go back to 2 on the same `SHA`.

## 5. Before each push
1. Run the tests related to the fix, scoped as in `test-scope.md`.
2. `Skill(znf:review)` with `BASE` = the head the bot last reviewed; fix what it finds in the same commit.
3. `git fetch`. If the remote branch moved, rebase the unpushed fix commit onto it; never force-push. The head the bot reviewed stays an ancestor, so `BASE` still resolves.
4. Push, set `SHA` to the new head, go back to 2.

## 6. Stop
- The check is green → wait for the PR's other checks via ci-watch step 4 (with `--watch`, run the full `Skill(znf:ci-watch)` instead), then report.
- Still red after 3 rounds → stop and report each open finding and what was tried. Never merge.
