# Daily plan and end of day

Read when: the ledger has no section `## <date +%F>` (morning plan), or the day is ending.

## Morning plan

1. **Carry-over.** Read the most recent `## YYYY-MM-DD` section. Every line not `done` comes along:
   - `carry-over`, `queued`, `failed`, a `running` line with no report → carry-over for today;
   - `blocked` → "Needs you" with its question, never dispatched until answered;
   - `deferred until <YYYY-MM-DD>` → carry-over when that date ≤ `date +%F` (compared when the plan
     is built); a later date is hidden from the plan but still copied.
2. **Gather the backlog** from the sources below.
3. **Filter, size and order** it; read `estimates.md` for calibration and the batch budget (see the
   estimation reference).
4. **Print the plan and wait for the user's approval or edits.** No worker is dispatched before
   the user approves. `ZNF_AUTONOMOUS=1` → record a `Ruling:` line in the ledger instead of
   stopping (`znf:_shared/autonomous.md`).
5. **Write today's section** `## <date +%F>`: the approved plan lines (status `queued`), plus every
   non-`done` line copied from the previous section that is not in the plan — still-future
   `deferred until`, backlog beyond Batch 1, `blocked`. Then dispatch Batch 1 only.

### Sources, in this order

1. Carry-over, including `deferred until` lines whose date has come.
2. Tasks the user gave as arguments when calling the skill.
3. The user's open PRs (`gh pr list --author @me --state open` per repo) with red CI or unresolved
   review threads → tasks; CLEAN and waiting for merge → "Waiting to merge" only, never dispatched.
4. Unfinished SDD ledgers `.znf/sdd/*/progress.md`.
5. Issues assigned to the user — only with the session line `features: github.issues on` (below).
6. Kanban items, `- [ ]` lines in `TODO.md` / `BACKLOG.md` / plan files.
7. All empty → dispatch one worker to run `znf:top-issues`; its top 5 go to "Needs you" as "pick one".

### Priority

prod broken > CI red > review comments > unfinished SDD > bug > approved feature > kanban > cleanup.
Same level → carry-over first, then the older task first.

### Filters

- A task that already has an open PR or a worktree (`gh pr list --state open`, `git worktree list`)
  → write "already in progress at <full URL>"; do not plan it again.
- A task that needs a user decision → "Needs you"; never dispatched.
- `deferred until` a date still in the future → hidden.
- Two tasks in the same repo touching the same files → mark both `sequential`.
- Estimate XL → mark "should split" before it is dispatched.

### Plan layout

Each line: `# · repo · task · source · issue · size · skill · parallel/sequential`.

```
Batch 1 (budget: <P> points from estimates.md with ≥ 3 days of data | else 10 tasks)
  1 · <repo> · <task> · carry-over · #12 · M · znf:fix · parallel
Needs you
  - <question> — <full URL>
Waiting to merge
  - <full PR URL> — ! gh pr merge -R <owner/repo> <n> --merge
Backlog: <count> more tasks
Calibration: <rule and the history lines it rests on, or "none yet">
```

When Batch 1 is done, pull the next tasks from the backlog in the same order without asking again.

## GitHub issues

Only when the session has a line starting `features: github.issues on`:

- Source 5: for each org among the workspace repos' `origin` remotes (`git -C <repo> remote get-url
  origin`), run `gh search issues --assignee=@me --state=open --owner=<org>`; keep issues of repos
  in the workspace.
- The ledger and plan `issue` column carries `#N` (or `owner/repo#N` across repos).
- The worker brief carries `Issue: #N`, which ship uses as the issue to close. A task with no issue
  → `Issue: -`; ship finds or creates one.

Toggle off → the issue column is `-` and no issue command runs.

## End of day

Run when the user says stop or end of day, or the backlog is empty with nothing actionable.

1. Done tasks, each with its full PR/issue URL; every CLEAN PR with its line
   `! gh pr merge -R <owner/repo> <n> --merge` (from the orchestrator's ci-watch result).
2. `blocked` tasks, each with its exact question.
3. Open CI watchers still running.
4. Estimate stats per the estimation reference: hit rate, bias, throughput, and the weekly warning.
5. Rewrite every `queued`, `running` or `failed` line in today's section as `carry-over` (keep its
   id, repo, issue and evidence), or as `deferred until <YYYY-MM-DD>` when the user names a date.
   `blocked` and `deferred until` lines stay as they are. Tomorrow's morning plan starts from these
   lines.
