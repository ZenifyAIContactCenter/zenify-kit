---
name: ci-watch
description: Watch the GitHub Actions runs of the commit just pushed, in the background, and wake the session when they finish — on a failure, read the log, fix it on the feature branch, push and watch again. Use when the user runs /znf:ci-watch or asks to watch or wait for CI.
allowed-tools: Bash(git rev-parse*) Bash(git branch --show-current) Bash(git status*) Bash(gh repo view*) Bash(gh run list*) Bash(gh run view*)
---

**Watch, then fix without asking — on a feature branch only.** This skill waits for CI and, on a
failure, fixes it and pushes again on its own. It never touches a deploy branch, never re-runs,
cancels or approves a workflow, and never merges.

## Step 1: Pin the commit

In the repo the task is working in (a worktree when there is one):

- `git branch --show-current` and `git rev-parse HEAD`; repo from `gh repo view --json nameWithOwner -q .nameWithOwner`.
- The commit must be on the remote: `git rev-parse @{u}` equals `HEAD`. No upstream, or the
  upstream differs → say so and stop; CI only runs on a pushed commit.
- The user named a branch, PR or sha → watch that commit instead (`gh pr view <n> --json headRefOid`).

## Step 2: Start the watcher in the background

Run the loop below as a **background command that wakes the session when it exits** (see
`_shared/harness-tools.md`, "Run a command in the background and be woken when it exits"),
with `REPO` and `SHA` filled in. Then tell the user in one line what is being watched and end the
turn; do not poll it yourself.

```bash
REPO=<owner/repo>; SHA=<full sha>; appear=$((SECONDS+300)); deadline=$((SECONDS+5400)); errs=0
while :; do
  if runs=$(gh run list -R "$REPO" --commit "$SHA" -L 50 --json databaseId,workflowName,status,conclusion,url \
      --jq '.[] | [.databaseId, .status, (.conclusion // "-"), .workflowName, .url] | @tsv'); then errs=0
  else errs=$((errs+1)); [ "$errs" -lt 3 ] || { echo "ci-watch: gh run list failed 3 times in a row"; exit 2; }; sleep 20; continue; fi
  if [ -z "$runs" ]; then
    [ "$SECONDS" -lt "$appear" ] || { echo "ci-watch: no workflow run for $SHA after 5 min"; exit 3; }
  elif ! printf '%s\n' "$runs" | cut -f2 | grep -qvx completed; then
    printf '%s\n' "$runs"
    printf '%s\n' "$runs" | cut -f3 | grep -qvxE 'success|skipped|neutral' && exit 1 || exit 0
  fi
  [ "$SECONDS" -lt "$deadline" ] || { echo "ci-watch: still running after 90 min"; printf '%s\n' "$runs"; exit 4; }
  sleep 20
done
```

Output is one line per run: `id  status  conclusion  workflow  url`. Exit codes: 0 all green,
1 a run did not succeed, 2 `gh` kept failing, 3 no run appeared, 4 timed out.

## Step 3: When the watcher wakes the session

- **0** → one line: the workflows that passed, with the sha. Stop.
- **1** → for each run whose conclusion is `failure`, `timed_out` or `startup_failure`:
  `gh run view <id> --log-failed`, read the failing step's last lines (not the whole log), and
  name the root cause with `file:line` where the log gives one. A `cancelled` run is reported, not
  diagnosed: it is usually a newer push replacing it. Then, without asking:
  - The branch is a deploy branch (the workspace's list, or the git guard refuses) → stop and
    report the cause; do not fix there.
  - Otherwise fix through `znf:fix` in the same worktree, commit, push the feature branch, and go
    back to Step 1 for the new commit.
  - Stop and hand back to the user after **2** fix rounds that did not turn CI green, or when the
    cause is outside the repo (a secret, a runner, an external service, a flaky test you cannot
    make deterministic). Report what each round changed and the failure still left.
- **2, 3, 4** → say which, quote the watcher's last line, and stop. Exit 3 often means the repo
  has no workflow for this branch or event.

## Rules

- One watcher per commit. A new push makes the old watch stale: start a new one for the new sha.
- Never re-run, cancel or approve a workflow, and never merge, from this skill.
- A fix stays within the failure: no refactor, no unrelated cleanup, no weakened or skipped test
  to make CI pass.
- No fabrication: a cause is named only from the log you read; "the log does not say" is an answer.
