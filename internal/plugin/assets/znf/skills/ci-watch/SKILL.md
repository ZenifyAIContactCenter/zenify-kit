---
name: ci-watch
description: Watch the GitHub Actions runs of the commit just pushed, in the background, and wake the session when they finish — on a failure, hand the failed run to znf:fix without asking, then watch the new commit. Use when the user runs /znf:ci-watch or asks to watch or wait for CI.
allowed-tools: Bash(git rev-parse*) Bash(git branch --show-current) Bash(git status*) Bash(gh repo view*) Bash(gh run list*) Bash(gh pr view*)
---

**Watch and hand off — never fix here.** This skill only waits for CI and wakes the session. On a
failure it hands the failed run to `znf:fix`, which reads the log, fixes and ships through
`znf:ship`'s gate; this skill then watches the new commit. It reads no log, edits no file and
pushes nothing itself.

## Step 1: Pin the commit

In the repo the task is working in (a worktree when there is one):

- `git branch --show-current` and `git rev-parse HEAD`; repo from `gh repo view --json nameWithOwner -q .nameWithOwner`.
- The commit must be on the remote: `git rev-parse @{u}` equals `HEAD`. No upstream, or the
  upstream differs → say so and stop; CI only runs on a pushed commit.
- The user named a branch, PR or sha → watch that commit instead (`gh pr view <n> --json
  headRefOid,headRefName`). Record its branch; a bare sha has none. Only a target whose branch is
  the current worktree's branch may be handed to fix in Step 3 — any other target is report-only.
- The repo has no `.github/workflows/*.yml` or `*.yaml` → there is no CI to watch; say so and stop.
- Invoked from `znf:ship --watch` with several repos → one watcher per repo.

## Step 2: Start the watcher in the background

Run the loop below as a **background command that wakes the session when it exits** (see
`_shared/harness-tools.md`, "Run a command in the background and be woken when it exits"),
with `REPO` and `SHA` filled in. Then tell the user in one line what is being watched and end the
turn; do not poll it yourself.

```bash
REPO=<owner/repo>; SHA=<full sha>; appear=$((SECONDS+300)); deadline=$((SECONDS+5400)); errs=0; last=
while :; do
  if runs=$(gh run list -R "$REPO" --commit "$SHA" -L 1000 --json databaseId,workflowName,status,conclusion,url \
      --jq '.[] | [.databaseId, .status, (.conclusion // "-"), .workflowName, .url] | @tsv'); then errs=0
  else errs=$((errs+1)); [ "$errs" -lt 3 ] || { echo "ci-watch: gh run list failed 3 times in a row"; exit 2; }; sleep 20; continue; fi
  n=$(printf '%s' "$runs" | grep -c .)
  [ "$n" -lt 1000 ] || { echo "ci-watch: 1000+ runs for $SHA, result unknown"; exit 5; }
  if [ "$n" -eq 0 ]; then
    [ "$SECONDS" -lt "$appear" ] || { echo "ci-watch: no workflow run for $SHA after 5 min"; exit 3; }
  elif ! printf '%s\n' "$runs" | cut -f2 | grep -qvx completed; then
    printf '%s\n' "$runs" | cut -f3 | grep -qvxE 'success|skipped|neutral' && { printf '%s\n' "$runs"; exit 1; }
    # Green only once the run set has held still for 60s: a pull_request run can appear after a push run finished.
    [ "$n" = "$last" ] && [ "$SECONDS" -ge "$settle" ] && { printf '%s\n' "$runs"; exit 0; }
    [ "$n" = "$last" ] || { last=$n; settle=$((SECONDS+60)); }
  else last=; fi
  [ "$SECONDS" -lt "$deadline" ] || { echo "ci-watch: still running after 90 min"; printf '%s\n' "$runs"; exit 4; }
  sleep 20
done
```

Output is one line per run: `id  status  conclusion  workflow  url`. Exit codes: 0 all green,
1 a run did not succeed, 2 `gh` kept failing, 3 no run appeared, 4 timed out, 5 too many runs.

## Step 3: When the watcher wakes the session

- **0** → one line: the workflows that passed, with the sha. Stop.
- **1** → the runs whose conclusion is `failure`, `timed_out` or `startup_failure` are the failed
  runs; a `cancelled` run is only reported (usually a newer push replaced it). Then, without asking:
  - The target is report-only (Step 1), or its branch is a deploy branch (the workspace's list) →
    report the failed run URLs and stop.
  - Otherwise invoke `Skill(znf:fix)` with the failed run URLs and the line
    `CI round <n>/2 for <branch>` (n = 1 on the first failure of this watch). A PR usually exists
    for the branch already: when ship's `gh pr create` says so, the push has updated that PR — do
    not open another.
  - After fix has pushed, go back to Step 1 for the new commit, round n+1.
  - Round 2 still red, or fix stops without a push (cause outside the repo, escalation to cook) →
    stop and hand back: each round's failed run and what fix changed.
- **2, 3, 4, 5** → say which, quote the watcher's last line, and stop. Exit 3 often means the repo
  has no workflow for this branch or event.

## Rules

- One watcher per commit. A new push makes the old watch stale: start a new one for the new sha.
- Never re-run, cancel or approve a workflow, and never merge, from this skill.
- Diagnosis, the fix and the push belong to `znf:fix` and `znf:ship`; do not duplicate them here.
