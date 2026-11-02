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

- Step 0: `zenify track start --kind fix --repo <repo> --est -`; not when run as part of `znf:hotfix` (see hotfix).
- After Step 2 (cause verified): `zenify track est <id> <S|M|L>`; not when run as part of `znf:hotfix`.
- Step 7 Report: `zenify track fix <id> --round <n> --path narrow|wide --regression test|no-seam|-` (`n` = the ROUND counter, `no-seam` = no regression test was possible).
- Step 8: `znf:ship` records the outcome and the PR.
- A `znf:fix` started by `znf:ci-watch` opens its own new `fix` id; it never reuses the id of the run that was being watched.
- When your args carry a `CI round <n>/2 for <branch>` line, forward that line verbatim to `znf:ship`.

## hotfix

- Step 1: `zenify track start --kind hotfix --repo <repo> --est <S|M|L>`.
- Hotfix step 3 borrows `/fix` steps 0-2: do not run the fix section's `start` or `est` there. The hotfix id from step 1 (started with its est) is the only id.
- `znf:ship` called from hotfix records `track ship` but never calls `end`.
- Code-fix path, step 10: `zenify track fix <id> --round <n> --path narrow|wide --regression test|no-seam|-`, then after the push `zenify track end <id> --branch <hotfix branch> --rounds <n>`. A multi-repo hotfix passes one `--branch` (wt gives every repo the same branch name).
- Revert/disable path (step 4 "Decide the response": revert or disable → skip to step 10): no `track fix`; close with `zenify track end <id> --rollback --rounds 0`.
- The hotfix id is closed only at step 10 (or on the rollback path).

## ship

- At the end of each run (shipped or stopped): `zenify track ship <id> --outcome shipped|blocked [--blocked-step lint|build|contract|behaviour|review] --findings C,H,M,L|- --rounds <n>`.
- `--blocked-step` is the first ❌ board line: `lint`, `build`, `contract gate` → `contract`, `behaviour`, `review`. A ship blocked by `zenify ui-verify check` is `--blocked-step behaviour`. Omit it when shipped.
- `--findings` = CRITICAL,HIGH,MEDIUM,LOW counts from Step 5 review (`-` when review did not run); `--rounds` = fix-loop rounds taken.
- After `gh pr create` for every repo (not when called from hotfix): `zenify track end <id> --pr <url> [--pr <url>…] [--ledger <SDD progress.md if one exists>] --rounds <n>`.
- When ship opens PRs and the feature is on, treat it as `--watch`: invoke `Skill(znf:ci-watch)` once per repo, with a line `track id: <id>` in the args.
- When your args carry a `CI round` line, do NOT invoke `Skill(znf:ci-watch)`: ci-watch re-watches the new commit itself (its Step 3).

## ci-watch

- The id comes from a `track id: <id>` line in the Skill args. At its Step 3, when the watched run finishes: `zenify track ci <id> --result green|red`, once per watched commit, before the fix handoff. Keep the original `track id:` for every round: `track ci <original id>` for each watched commit, including commits pushed by the fix it handed off to (that fix still opens its own `fix` id).
- `report-only` targets are recorded the same way; ci-watch still never fixes them.
- No `track id:` line given → call nothing.

## Backlog

- Work that comes from the backlog: use `start --from <bid|issue url>`, i.e. `zenify track start --kind <cook|fix|hotfix> --repo <repo> --est - --from <bid|issue url>`; the item leaves the backlog. An unknown or dropped `bid` exits non-zero and writes nothing.
- Work that surfaces during a task and is not done now, including a minor issue you do not fix in the current task: `zenify track backlog add "<title>" --repo <repo> [--est S|M|L] [--ref <issue url|memory:<name>>]`; it prints the `bid`.
- Drop an item that is no longer wanted: `zenify track backlog drop <bid>`.

## Onboard

Run this when the digest says `track: not onboarded`.

1. Read the project's `MEMORY.md` at `~/.claude/projects/<workspace path with '/' replaced by '-'>/memory/MEMORY.md`, then every memory it links with `type: project`.
2. List the still-open work to the user. Add items to the backlog only after the user approves, and only the approved ones: `zenify track backlog add "<title>" --repo <repo> --ref memory:<name>`. Never write the backlog before approval.
3. Offer past-session history: preview with `zenify track onboard history`, and run `zenify track onboard history --write` only when the user agrees.
4. Last, `zenify track onboard memory`; it marks the workspace onboarded.
