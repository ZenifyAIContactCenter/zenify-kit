<!-- Moved verbatim from cook/SKILL.md § Step 6, First the worktree, § Then hand the slug its own
workspace (W4 slim-skills). Read when: $HERDR_WORKSPACE_ID is set (workspace handoff recipe) or
before a second worktree. -->

### From § First, the worktree — why spec/plan and paths must be absolute (extra, tier two)

Spec and plan are gitignored, so they do not follow a worktree, and `wt rm` would delete them. A
relative path resolves inside the worktree and finds nothing.

### From § First, the worktree — no decision (extra, tier two)

Nothing inspects `git status --porcelain` to decide, because there is no decision.

### From § First, the worktree — the per-repo `Waits for:` edges (extra, tier two)

The per-repo `Waits for:` edges come from the plan.

### From § First, the worktree — re-entering `/cook` (extra, tier two)

The slug does not get re-derived from whatever the latest message was about.

### From § First, the worktree — this is the step that writes code

**Fetch again even though Step 0 fetched — this is the cook-specific reason.** The base moved while
brainstorming and planning happened, which is real time; that gap is exactly what the Step-0/Step-6
split introduced. Keep the explicit fetch above; why it stays load-bearing across `wt` builds is the
base-ref rule in znf:discipline §8.

`znf:subagent-driven-development` Setup will otherwise create a worktree of its own and
assumes a single repo. Tell it the workspace already exists so it only verifies.

### Then hand the slug its own workspace — once, and only from outside it

Steps 0-5 belong in the workspace you started in: it holds the spec, the plan, and `/gate`. Step 6
onward belongs somewhere that is *this slug and nothing else*, with the repo's dev servers beside it.
When herdr is running, move the work there. Skip this whole section when `$HERDR_WORKSPACE_ID` is
unset — everything below is an optimisation of where work happens, never a precondition for it.

**The loop guard comes first, because getting it wrong breaks this in one of two directions.** The
session that receives the handoff re-enters `/cook` with a plan file and arrives back here; it must
not hand off again. The condition is **"am I in the workspace bound to this worktree?"**, which only
`herdr worktree open` ever sets:

```bash
HERE=$(herdr workspace get "$HERDR_WORKSPACE_ID" \
        | node -pe 'const w=JSON.parse(require("fs").readFileSync(0)).result.workspace; w.worktree ? w.worktree.checkout_path : ""')
[ "$HERE" = "$(cd <repo> && wt path <slug>)" ] && echo SKIP      # already home → straight to SDD
```

**The obvious guard is wrong, and it failed in exactly this way on the first real run.** It was
`git config --get wt.slug` — non-empty meaning "already inside the worktree". But `wt.slug` is
worktree-scoped git config, and the block immediately above this one tells you to `cd` into the
worktree and work there. So by the time this guard is read, cwd is inside the worktree, `wt.slug` is
set, and **the handoff is skipped by the very session that just created the worktree** — deterministically, not by luck. Observed: 11 worktrees in `contact-center-hub`, each with `wt.slug`
set, a session working inside one of them, and no per-slug workspace ever opened.

A workspace created by `workspace create` carries **no** `worktree` field, so this test also
distinguishes "a workspace whose cwd happens to be the repo" from "the workspace of this slug".
`$HERDR_WORKSPACE_ID` unset → no herdr → skip the section entirely, as below.

```bash
ORIG="$HERDR_WORKSPACE_ID"                              # where the spec and plan live
W=$(herdr worktree open --cwd "<repo>" --path "$(wt path <slug>)" \
      --label "<slug>" --no-focus --json | node -pe 'JSON.parse(require("fs").readFileSync(0)).result.workspace.workspace_id')
P=$(herdr pane list --workspace "$W" --json | node -pe '...root pane id...')
herdr agent start "<slug>" --kind claude --pane "$P"
herdr workspace focus "$ORIG"                           # unconditionally — see below
herdr agent prompt "<slug>" "/cook <ABSOLUTE path to the plan file>"
```

Four things that each cost a wrong turn earlier:

- **`--no-focus`, then `workspace focus "$ORIG"` anyway.** `agent start` has no `--no-focus`, and
  whether it steals focus measured *differently on two runs*. Restoring unconditionally is correct
  either way and costs one call. Not focusing is the right default regardless: with several repos in
  flight, any automatic choice of where to look is wrong, and only the user knows which repo matters.
- **The plan path must be absolute.** The new session's cwd is the worktree; a relative path resolves
  there and the plan does not exist there — rule #8 keeps it in the main checkout.
- **`agent prompt` after `focus`,** so the prompt lands in a session that is already settled.
- **This is a handoff, not a migration.** The running conversation cannot move — `claude --resume`
  needs the old session dead first, and the session issuing these commands is the one that would have
  to die. What crosses over is the plan file, which `writing-plans` already requires to be
  self-contained. If the new session cannot work from it, the plan was incomplete, and finding that
  out is worth more than the convenience it costs.

Then **stop**. Report the workspace, the pane and the branch, and let the handed-off session run
Steps 6-7. Do not also run SDD here: two sessions on one worktree is the contaminated-review case
this skill spends a whole section forbidding.
