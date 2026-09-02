---
name: sweep
description: Tear down a finished task — stop its dev servers, close its herdr workspaces, remove its worktrees and branches, across every repo it touched. Use when work has landed and the workspace should go back to clean, or when asked to clean up / dọn dẹp after a task. Refuses to report success when nothing has actually merged yet, and says what is still needed instead.
allowed-tools: Bash(wt *) Bash(git *) Bash(herdr *) Bash(hcall *) Bash(node *) Bash(ls *) Read
---

**The destructive work is `wt sweep`, not this file.** Everything that stops a process or deletes a
worktree happens inside that subcommand, which calls `wt rm` **without** `--force` — so the gate is
`wt rm`'s existing refusals (dirty tree, detached HEAD, no merge trace in base) and nothing here may
work around them. This skill exists for the three things a subcommand cannot do: find the repos, judge
whether it is even *time* to sweep, and report across them.

Fastest path when you already know it is time and it is one repo: **`!wt sweep --fetch`** typed
straight into the session, no agent turn at all. Use this skill when the task spanned repos, when you
are not sure the work has landed, or when you simply said "clean up".

## Step 1: Find the task's worktrees, in every repo

The slug is the same in every repo by house rule #8, so one name finds them all:

Run it from the workspace root — the directory holding the repos, which is the current project's
own layout and not something this skill should assume:

```bash
for r in */; do
  [ -f "$r/.claude/worktree.json" ] || continue
  git -C "$r" worktree list --porcelain 2>/dev/null | grep -q "/.worktrees/<slug>$" \
    && echo "$r"
done
```

A single-repo project has nothing to loop over: `wt sweep` in that one repo is the whole job.

No slug given → sweep every finished task instead, which is the same command with no filtering. Say
which of the two you are doing; they differ in blast radius and the user may have meant either.

## Step 2: Is it time? — the gate a bare `wt sweep` cannot express

`wt sweep` removes only what is **merged**. Run before the merge it prints `0 removed` and exits 0 —
which reads as "nothing to do, all clean" when the truth is "not yet, and here is why". Distinguish
these before running anything:

```bash
git -C <repo> fetch origin --quiet
git -C <repo> log --oneline origin/<base>..<branch> | wc -l    # commits not in base
git -C <repo> log --oneline <branch>..origin/<base> | wc -l    # base ahead
```

| State | What to say |
|---|---|
| branch not pushed | `/ship` first — nothing to merge yet |
| pushed, no PR / PR open | **the merge is the user's** (rule #7). Report the branch and stop |
| merged | proceed |
| **hotfix, merged into `release<N>` only** | **stop.** The sync back to `staging` comes first, or next week's release silently loses the fix. `/hotfix` step 9 owns this |

That last row is the one worth stopping for: removing the worktree and branch of an un-synced hotfix
destroys the only local copy of a fix that is about to be lost, and it looks like a successful cleanup.

## Step 3: Sweep, per repo

```bash
(cd <repo> && wt sweep --fetch)
```

`--fetch` is not optional here. `wt sweep` compares against `origin/<base>`, a **local**
remote-tracking ref: merge the PR on the server without fetching and that ref is stale, the merged
branch is not yet its ancestor, and sweep keeps everything while looking like it worked.

Do not pass `--force` to `wt rm` yourself to "help" a refusal along. Every refusal is information:
uncommitted changes mean someone's work is in there, and no merge trace means it has not landed.

## Step 4: Report what was left, not just what went

```
<repo>   swept: <slug> (port <n> stopped, workspace closed)
<repo>   kept:  <slug> — merged but has uncommitted changes
```

The kept lines are the useful half. A sweep that removed 2 of 5 and reported only the 2 hides three
worktrees that need a decision.

**One thing `wt sweep` will not do, by design:** it never closes the herdr workspace it is *running
in*, because that would kill the shell mid-sweep and the removal after it would never happen. It says
so; that pane is yours to close.

## Red flags

| Thought | Reality |
|---|---|
| "`0 removed` means everything is clean" | It usually means nothing has merged yet. Check, then say which. |
| "I'll `--force` past the refusal" | The refusals are the entire safety gate. Report them instead. |
| "The hotfix is merged to release, so it's done" | Not until it is synced back to `staging`. Sweeping now loses the fix. |
| "One repo swept, task done" | The slug exists in every repo it touched. Loop them. |
| "I should reimplement this in the skill for speed" | `wt sweep` is tested (16 assertions). Prose reimplementing it is not. |
