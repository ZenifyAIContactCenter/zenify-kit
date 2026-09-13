<!-- Moved verbatim from cook/SKILL.md § Step 6, First the worktree (W4 slim-skills). Read when:
before a second worktree, or when SDD Setup wants to create its own. The per-user terminal-workspace
handoff that used to live here is a personal skill, not part of the kit. -->

### From § First, the worktree — why spec/plan and paths must be absolute (extra, tier two)

Spec and plan are gitignored, so they do not follow a worktree, and `wt rm` would delete them. A
relative path resolves inside the worktree and finds nothing.

### From § First, the worktree — no decision (extra, tier two)

Nothing inspects `git status --porcelain` to decide, because there is no decision.

### From § First, the worktree — the per-repo `Waits for:` edges (extra, tier two)

Independent repos run concurrently from the start. The per-repo `Waits for:` edges come from the
plan.

### From § First, the worktree — the worktree-created-at-Step-6 fetch/comment detail (extra, tier two)

The `git fetch` immediately before `wt new` is belt-and-suspenders: current `wt` fetches too, but
older builds do not.

### From § First, the worktree — re-entering `/cook` (extra, tier two)

The slug does not get re-derived from whatever the latest message was about.

### From § First, the worktree — this is the step that writes code

The worktree belongs *here*, not at Step 0: the plan is agreed, so this is the moment the first
line of repo code gets written.

**Fetch again even though Step 0 fetched — this is the cook-specific reason.** The base moved while
brainstorming and planning happened, which is real time; that gap is exactly what the Step-0/Step-6
split introduced. Keep the explicit fetch above; why it stays load-bearing across `wt` builds is the
base-ref rule in znf:discipline §8.

`znf:subagent-driven-development` Setup will otherwise create a worktree of its own and
assumes a single repo. Tell it the workspace already exists so it only verifies.
