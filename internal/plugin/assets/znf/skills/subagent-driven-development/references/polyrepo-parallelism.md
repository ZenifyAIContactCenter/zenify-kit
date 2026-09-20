<!-- Moved verbatim from subagent-driven-development/SKILL.md § The Task Loop (W4 slim-skills). Read when: the plan spans repos and you need the dependency and ledger rules. -->

**Cross-worktree parallelism (polyrepo).** A feature that spans repos runs as several
worktrees, one per repo (same slug) — the conflict rule above is about writers in the SAME
worktree; across repos, each has its own git working directory and index, so two implementers
writing into two repos cannot collide. Run them in parallel.

- Read the plan's per-repo `Repo:` / `Waits for:` block (writing-plans authors it). Dispatch
  **one implementer per repo**, concurrently for every repo whose dependency edges are all
  satisfied. A repo with no `Waits for:` line is independent — start it immediately.
- **Gate on contract-freeze, not on the whole repo.** A repo whose block says
  `Waits for: <X> : contract-frozen` starts as soon as X commits the endpoint + shape (a
  specific commit you record), NOT after X's whole plan finishes.
- **One ledger, repo-tagged.** Keep the single plan ledger; tag each task line with its repo
  (`[be] Task 3: complete`). Each repo's review loop runs on its own stream.
- **Collect each stream by name** — you must collect each repo-stream's report and treat
  silence as incomplete, never as clean (CLAUDE.md §3). Keep one ledger line per repo-stream.
  A stream that went quiet is NOT done.
- **Within one repo, tasks stay sequential** — the rule above is unchanged; only the
  across-repo case is the exception.
