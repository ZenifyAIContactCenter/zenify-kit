# Worker brief and routing table

Read when: dispatching a worker.

Workers are sub agents and **cannot spawn agents**. Tell each one to run its znf skill and do inline
any step that skill would normally dispatch. The one exception is ship's step 5 (independent
review): the orchestrator runs it.

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
Ship: stop before step 5 (independent review). Commit on the feature branch, do not push, and
            return READY_FOR_REVIEW. When resumed clean: finish ship with the review recorded as
            done by the orchestrator's znf:code-reviewer — board, push, open the PR — and return
            DONE with the PR URL. Resumed with findings: fix them, re-run steps 1-4, report again.
            Do not run znf:ci-watch; the orchestrator watches CI for your PR.
Return ≤ 40 lines: status (DONE | READY_FOR_REVIEW | BLOCKED | FAILED), PR URL (DONE), worktree
            path, base ref, ship fingerprint, steps 1-4 evidence, commits, fix/review rounds,
            commands + real output, question if BLOCKED, new tasks found. Long output → file path.
```

This holds for every route, "implement directly" included (it runs `znf:ship` the same way).

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
| clear task, one repo, what + why stated | implement directly, then `znf:ship` |
| merged PR found | `znf:sweep` |
| empty backlog | `znf:top-issues` (report only; the user picks) |
