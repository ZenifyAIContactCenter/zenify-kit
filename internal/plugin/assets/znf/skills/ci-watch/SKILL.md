---
name: ci-watch
description: Watch the GitHub Actions runs of the commit just pushed, in the background, and wake the session when they finish — on a failure, hand the failed run to znf:fix without asking, then watch the new commit. Use when the user runs /znf:ci-watch or asks to watch or wait for CI.
allowed-tools: Bash(git rev-parse*) Bash(git branch --show-current) Bash(git status*) Bash(gh repo view*) Bash(gh run list*) Bash(gh pr view*) Bash(gh pr checks*) Bash(gh api graphql*)
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
- The repo has no `.github/workflows/*.yml` or `*.yaml` → there is no CI to watch; say so. The
  branch has an open PR (`gh pr view <branch> -R "$REPO" --json number,state`, state `OPEN`) →
  go to Step 4; otherwise stop.
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

- **0** → one line: the workflows that passed, with the sha. The branch has an open PR (as in
  Step 1) → Step 4; otherwise stop.
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
  has no workflow for this branch or event; then an open PR for the branch → Step 4 instead of stopping.

## Step 4: Wait for the PR to be mergeable

Green CI is not a mergeable PR: required reviews, review threads and checks from other apps still
gate it. Run this loop in the background exactly like Step 2, with `REPO` and the PR number `PR`:

```bash
REPO=<owner/repo>; PR=<number>; deadline=$((SECONDS+3600)); errs=0; st=
while :; do
  if st=$(gh pr view "$PR" -R "$REPO" --json mergeStateStatus,reviewDecision,url \
      --jq '[.mergeStateStatus, .reviewDecision, .url] | @tsv'); then errs=0
  else errs=$((errs+1)); [ "$errs" -lt 3 ] || { echo "ci-watch: gh pr view failed 3 times in a row"; exit 7; }; sleep 30; continue; fi
  gh pr checks "$PR" -R "$REPO" >/dev/null 2>&1; rc=$?   # 8 = checks still pending
  case "$st" in
    CLEAN*) echo "$st"; exit 0 ;;
    DIRTY*) echo "$st"; exit 3 ;;
    BEHIND*) echo "$st"; exit 4 ;;
    DRAFT*) echo "$st"; exit 5 ;;
    UNSTABLE*) [ "$rc" = 8 ] || { echo "$st"; exit 1; } ;;
    BLOCKED*) [ "$rc" = 8 ] || { echo "$st"; exit 2; } ;;
  esac   # UNKNOWN, HAS_HOOKS, or checks pending → poll again
  [ "$SECONDS" -lt "$deadline" ] || { echo "ci-watch: PR not mergeable after 60 min"; echo "$st"; exit 6; }
  sleep 30
done
```

The last line is `mergeStateStatus  reviewDecision  url`. When it wakes the session:

- **0** → the full PR URL and the merge command for the user: `! gh pr merge -R <owner/repo> <n> --merge`.
- **1** → the failing non-required checks (`gh pr checks <n> -R <owner/repo>`), the PR URL and the
  same merge command; the user decides.
- **2** → list what blocks it. Unresolved threads:
  `gh api graphql -f o=<owner> -f r=<repo> -F n=<n> -f query='query($o:String!,$r:String!,$n:Int!){repository(owner:$o,name:$r){pullRequest(number:$n){reviewThreads(first:100){nodes{isResolved path comments(first:1){nodes{url}}}}}}}'`
  → each node with `isResolved: false`, as path + comment URL. No unresolved thread → the
  `reviewDecision` and the failing required checks (`gh pr checks <n> -R <owner/repo> --required`).
  Do not resolve a thread and do not poll again; stop.
- **3** conflicts, **4** behind the base, **5** draft, **6** timed out, **7** `gh` kept failing →
  say which, quote the last line, and stop.

## Rules

- One watcher per commit. A new push makes the old watch stale: start a new one for the new sha.
- Never re-run, cancel or approve a workflow, never resolve a review thread, and never merge, from
  this skill.
- Diagnosis, the fix and the push belong to `znf:fix` and `znf:ship`; do not duplicate them here.
