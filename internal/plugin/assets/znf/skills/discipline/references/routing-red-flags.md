# Routing red flags — thoughts that mean you are rationalising

Each row is a thought that feels like judgement and is actually a way around § 0. Read it when you catch yourself explaining why a step does not apply here.

| Thought | Reality |
|---|---|
| "this is small, skip the ceremony" | The floor has four conditions. Check them; don't feel them. |
| "the user didn't ask for the skill" | Forgetting is normal. Catching it is your job, not theirs. |
| "I'm only investigating, not changing yet" | Correct — route at the first file you edit, not before. |
| "I'll route after it works" | Too late. The worktree must exist before the first edit. |
| "It's one file, the main checkout is fine" | Rule #8 has no size clause. Fetch and branch. |
| "I'll pipe the worktree command through a pager to keep it short" | A tool that exits early can send SIGPIPE and kill the worktree command mid-run, past the point where its own cleanup can fire — leaving a half-built worktree with no config. Redirect to a file and read the file instead. |
| "the base flag is enough" | Current builds fetch before resolving the base, but do not lean on that alone — an explicit `fetch` first stays correct on any build, and a local base branch is never guaranteed current: read `origin/<base>`. |
| "a spec for this is overkill" | The exact sentence a brainstorming/planning skill forbids. Short spec ≠ no spec. |
| "it looks local to me" | Judgement is not the gate. Run the gate. |
| "now I'm changing something else, so a new worktree" | One worktree per **slug**, not per edit. `cd` into the one already open; the tool will refuse a duplicate anyway. |
| "the plan has 6 tasks, so 6 worktrees" | No — plan tasks share the plan's one worktree. "Task" at the tool level means the slug. |
