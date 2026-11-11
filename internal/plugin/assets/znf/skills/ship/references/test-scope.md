# Ship step 4: test scope

## Scope
Run the tests affected by the branch against `$(zenify wt base)`, using the project's own test command, e.g. `npx vitest run --changed "$(zenify wt base)"`, `npx jest --changedSince="$(zenify wt base)"`, else the test files and packages the diff touches (`go test` of the changed packages).

## Hotfix
`git config --worktree --get wt.type` printing `hotfix` means run the full suite (CI on the hotfix base runs no unit tests yet). Empty or anything else means affected tests. Never infer hotfix by comparing `zenify wt base` with `baseRef`.

## Time
Run in the background, or with an explicit `timeout` longer than the suite; a run cut by the 2-minute tool limit has no result.

## Report
Give the command and the count of tests that ran; a run of zero tests is not a pass. No test covers the change → `Skill(znf:run)`, as step 4 says.

## Fix loop
The ship fix loop re-runs step 4 with this same scope, never wider.

## Why
The full suite belongs to CI on the PR.
