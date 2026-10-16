---
name: where
description: Recap where the current task stands — goal, done, in progress, next step, blockers — from the conversation plus the kit's real state (ledger, branch, worktree, PR). Use when the user asks "where are we", "status", "recap", "what is left", or resumes after a break or a compaction.
allowed-tools: Read Grep Glob Bash(git status*) Bash(git branch --show-current) Bash(git log*) Bash(gh pr view*) Bash(zenify wt ls*)
---

**Report, never act.** This skill reads state and prints one recap. It edits no file, runs no
mutating command, dispatches no agent, and starts no next step on its own.

## Step 1: Context

Read the conversation in context, including any compaction summary: the user's goal, decisions
made and options rejected, the spec/plan paths in play, the last command that ran and its result.

## Step 2: Real state (read-only, only what exists)

For each repo the task touched (no repo → skip this step):

- `git status --short`, `git branch --show-current`, `git log --oneline -5`; commits ahead of the
  base: `git log --oneline origin/<base>..HEAD`, base from the PR's `baseRefName` or the repo's
  worktree config; no fetch, so the count is as of the last fetch.
- Worktree: `zenify wt ls` in that repo — which slug, running or merged.
- Ledger: the SDD ledger at `<repo-root>/.znf/sdd/*/progress.md` matching the plan in play.
  Count `Task <N>: complete` against the plan's tasks; name the first task not complete.
- PR: `gh pr view --json number,url,state,isDraft,mergeable,baseRefName,statusCheckRollup` for the current
  branch. No PR → say "no PR yet".
- Dispatched agents: the ledger lines in `progress.md` (and the ship board, if any) first, then
  agents this session dispatched — reported, or still running.
  Silence is not a clean result: an agent with no report is **INCOMPLETE**, never "nothing found".

A command that fails or is unavailable → say which one and why, do not guess its answer.

## Step 3: Phase

Name where the task sits in the pipeline, from evidence in Steps 1–2:
branch → spec → ground → plan → implement (SDD) → ship (gate, review, push, PR) → merged → sweep.
The **next step** is the kit command for the next phase (`/znf:cook`, `/znf:ship`, `/znf:sweep`, …),
or the open ledger task. A gate that failed and was never re-run is the next step, not "done".

## Step 4: Answer

Reply in the user's language, short, in exactly this shape (drop a section that is empty):

```
**Goal:** <one line>
**Phase:** <phase> — <repo> @ <branch> (<worktree slug>)

**Done:**
- <item> — <evidence: commit sha, test command + result, PR #>

**In progress:**
- <item> — <what is left>

**Next:**
- <one concrete step, with the kit command>

**Blocked / your call:** <decision only the user can make>
```

## Rules

- No fabrication: list only what was done and verified, each with its evidence. A failing test,
  a skipped step or an unread agent report is stated plainly.
- State outranks memory of the conversation: if git, the ledger or the PR disagrees with what the
  conversation says, report the disagreement.
- Nothing in context and no task state → say there is no task yet, and stop.
- End with one question offering the next step when there is one. Do not start it.
