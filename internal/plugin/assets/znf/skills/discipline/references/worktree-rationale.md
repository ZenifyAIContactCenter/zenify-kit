<!-- Moved verbatim from discipline/SKILL.md § 8 — "Why unconditional" and "Where this cannot apply" (W4 slim-skills). Read when: you are about to argue the worktree rule does not apply here. -->

### Why unconditional, rather than "only for concurrency"

- It deletes a judgement call, and judgement calls are what get skipped when it matters. Deciding
  case-by-case needs a status check plus the current branch plus a decision; unconditional needs
  neither.
- It removes the stale-base failure entirely. Branching from a local base branch inherits however
  old that branch is; branching from a freshly fetched remote ref after a fetch cannot.
- A merged task does not advance your local base branch — the merge lands on the remote, not on
  the local ref your main checkout reads. So the next task re-fetches and reads `origin/<base>`;
  never treat a local base branch as current just because a previous task merged.
- A permanently clean main checkout turns any session-start git-state report into a real alarm.
  While several repos sit dirty, "dirty" is background noise; once it should never happen, it means
  something went wrong.
- The cost is smaller than it looks. Copy-on-write cloning of dependencies is near-instant and costs
  almost no disk until files diverge; symlinking is cheaper still. What genuinely costs is a second
  dev server (which you want anyway, to verify) and a worktree to tear down — a sweep command should
  do that teardown in one shot for every merged, clean task, and should refuse to remove work that
  has not landed. Without it, worktrees accumulate.

### Where this cannot apply

- **Not a git repo.** A plain config directory, or a polyrepo container directory that is not itself
  a repo. There is nothing to branch.
- **The file is gitignored.** A worktree for it is not just overhead, it is destructive: the file
  does not follow into the worktree, and tearing the worktree down deletes whatever was written
  there. Specs, plans and per-repo docs are usually in this category — check the repo's `.gitignore`
  rather than assuming either way.
- **The repo declares no worktree config.** The worktree tool should refuse outright rather than
  guess at ports, dependency handling, or which files to seed. Create a config modelled on a
  neighbouring repo, and give it isolation (a port range, etc.) that no other repo in the same
  workspace uses. Put any local settings and the repo's own docs in whatever the tool's "copy on
  create" list is, or every worktree silently writes state to a *separate* store with nothing to
  warn you.
- **The repo is not a Node project.** This used to be a carve-out; it is not one any more. `wt`
  reads `"deps"` from the repo's worktree config and `"none"` means "nothing to seed per-worktree"
  (Maven with a global `~/.m2`, Go modules, a Python venv you recreate, …). Give the repo a
  `.claude/worktree.json` with `"deps": "none"` and its own port block, and `wt new` works there
  like anywhere else. Do not fall back to a bare `git worktree add` or to a personal terminal tool:
  a worktree `wt` did not create is invisible to `wt ls`/`wt rm`/`wt sweep`, so it never gets torn
  down. If `wt` refuses because the config is missing, the fix is the config, not a different tool.

Also worth knowing before the first worktree of a session: current builds of the worktree tool fetch
before resolving the base, but older ones do not — so an explicit fetch first is still the safe habit,
and a local base branch is never assumed current regardless. The tool needs its config file, and the
directory it creates must be ignored, or the checkout you just cleaned goes dirty again with an
untracked worktree directory.

