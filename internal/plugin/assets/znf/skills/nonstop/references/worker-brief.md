# Worker brief and routing table

Read when: dispatching a worker.

Workers are sub agents and **cannot spawn agents**. Tell each one to run its znf skill and do inline
any step that skill would normally dispatch (including ship's review).

## Brief — every dispatch

```
Task:       <id> — <goal>, source <source>, size <S|M|L|XL>
Repo:       <absolute path> · worktree slug: <slug> (git fetch origin; wt new <slug> --type feat|fix --base <baseRef>)
Issue:      #N | -
Skill:      run Skill(<znf skill from the routing table>) for this task
Done when:  <done-criteria>
Rules:      znf:discipline floor — ground before use, verify with real output, conventional commits,
            feature branch only, push allowed, NEVER merge, NEVER dispatch agents.
            Issue: #N → ship uses it as the issue to close (Closes #N); Issue: - → ship's own rule.
Stop & report (do not guess) on: a user gate of the skill, a destructive or irreversible action,
            anything outside the worktree.
Do not run znf:ci-watch; the orchestrator watches CI for your PR.
Return ≤ 40 lines: status (DONE | READY_FOR_REVIEW | BLOCKED | FAILED), PR URL when ship opened
            one, worktree path, base ref, commits, fix/review rounds, commands + real output,
            question if BLOCKED, new tasks found. Long output → file path.
```

`DONE` = the route ended in `znf:ship` and the PR is open. `READY_FOR_REVIEW` = the "implement
directly" route: verified, committed, pushed, no PR yet; the orchestrator reviews, then resumes you
to run `znf:ship`.

The worktree path and base ref let the orchestrator run `git diff --shortstat <base>...HEAD`. The
worker does not report its own time; `duration_ms` comes from the harness.

## Routing table

| Task | Worker runs |
|---|---|
| bug / CI red / failing test | `znf:fix` |
| broken in production now | `znf:hotfix` |
| feature, no agreed design | `znf:cook` — stops at its first user gate |
| approved plan `.md` | `znf:cook <plan.md>` |
| PR review threads | `znf:fix` with the thread URLs; reply and resolve each after the fix |
| kanban item | the project's kanban skill when installed, else `znf:cook` |
| clear task, one repo, what + why stated | implement directly; `znf:ship` only after the orchestrator's review |
| merged PR found | `znf:sweep` |
| empty backlog | `znf:top-issues` (report only; the user picks) |
