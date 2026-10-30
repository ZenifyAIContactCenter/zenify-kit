# Track — record the run to `zenify track`

Read this when the session digest says `features: track on`. Every command below is `zenify track …`; run it from the task's worktree or the workspace.

## Rules

- `zenify track start` prints `track: <id>`. Keep that id for the whole task and pass it to every later call.
- Stdout `track: off` (from any track command) → skip every later track call this task.
- A track command that exits non-zero: report it in the final message; never retry blindly, never block the task on it.
- Only record; never change what a skill does.

## cook

- Step 0, before the base sync: `zenify track start --kind cook --repo <repo> --est -`; remember the id.
- Step 2, right after the design gate is approved: `zenify track est <id> <S|M|L>`.
- Step 7: call `znf:ship` as usual; ship records the outcome and the PR (see ship).

## fix

- Step 0: `zenify track start --kind fix --repo <repo> --est -`.
- After Step 2 (cause verified): `zenify track est <id> <S|M|L>`.
- Step 7 Report: `zenify track fix <id> --round <n> --path narrow|wide --regression test|no-seam|-` (`n` = the ROUND counter, `no-seam` = no regression test was possible).
- Step 8: `znf:ship` records the outcome and the PR.
- A `znf:fix` started by `znf:ci-watch` opens its own new `fix` id; it never reuses the id of the run that was being watched.

## hotfix

- Step 1: `zenify track start --kind hotfix --repo <repo> --est <S|M|L>`.
- `znf:ship` called from hotfix records `track ship` but never calls `end`.
- Code-fix path, step 10: `zenify track fix <id> --round <n> --path narrow|wide --regression test|no-seam|-`, then after the push `zenify track end <id> --branch <hotfix branch> --rounds <n>`.
- Revert/disable path (step 3 rollback decision): no `track fix`; close with `zenify track end <id> --rollback --rounds 0`.
- The hotfix id is closed only at step 10 (or on the rollback path).

## ship

- At the end of each run (shipped or stopped): `zenify track ship <id> --outcome shipped|blocked [--blocked-step lint|build|contract|behaviour|review] --findings C,H,M,L|- --rounds <n>`.
- `--blocked-step` is the first ❌ board line: `lint`, `build`, `contract gate` → `contract`, `behaviour`, `review`. Omit it when shipped.
- `--findings` = CRITICAL,HIGH,MEDIUM,LOW counts from Step 5 review (`-` when review did not run); `--rounds` = fix-loop rounds taken.
- After `gh pr create` for every repo (not when called from hotfix): `zenify track end <id> --pr <url> [--pr <url>…] [--ledger <SDD progress.md if one exists>] --rounds <n>`.
- When ship opens PRs and the feature is on, treat it as `--watch`: invoke `Skill(znf:ci-watch)` once per repo and pass the track id along.

## ci-watch

- When the watched run finishes: `zenify track ci <id> --result green|red`, once per watched commit, before the fix handoff.
- `report-only` targets are recorded the same way; ci-watch still never fixes them.
- Without an id (not started under track) → call nothing.
