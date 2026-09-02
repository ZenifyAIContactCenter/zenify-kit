---
name: prune-memory
description: Review and prune auto memory in one batch — remove task logs, dead references, rival memories, and misfiled scopes. Use when the SessionStart reminder fires, or when memory feels noisy or the index is growing.
argument-hint: "[optional: path to a memory dir, defaults to this project's]"
allowed-tools: Read Grep Glob Bash Edit Write
---

**Batch, not per-item.** Review everything in one pass and present one list of proposals. Never
ask about memories one at a time: approval habituation begins at the *second* prompt, so a
per-item gate becomes reflexive clicking and is then worse than no gate at all — it adds
friction and false assurance while changing nothing.

## Step 0: Locate the real memory directory

Do not assume `~/.claude/projects/<sanitized-cwd>/memory/`. It is redirected per-project:

```bash
grep -h "autoMemoryDirectory" .claude/settings.local.json .claude/settings.json 2>/dev/null
```

If set, that path wins (precedence is Local > Project > User). If not, the default is
`~/.claude/projects/<project>/memory/`, keyed by **git repo root** — so all worktrees and
subdirectories of one repo share it. Say which directory you are pruning before you touch it.

## Step 1: Inventory

```bash
M=<the memory dir>
wc -l "$M/MEMORY.md"; wc -c "$M/MEMORY.md"        # cap: 200 lines OR 25KB
ls -1 "$M"/*.md | wc -l                            # file count
grep -h "^  modified:" "$M"/*.md | sort | head     # oldest-touched first
```

`modified` is written automatically on every memory write, so it is a real staleness signal —
use it to order the review, oldest first. Report the index against its cap: content past
200 lines / 25KB is dropped **silently** on the next load.

## Step 2: Classify every memory into exactly one bucket

- **KEEP** — states a rule that changes a future decision, and is still true.
- **MERGE** — two or more memories are authoritative on one subject. Keep the one whose index
  line would actually get opened; fold the rest into it; delete the losers. Rival memories are
  the same defect as two skills disagreeing, and the loser is whichever one happens to be read.
- **FIX** — right subject, but names a file, function, agent, flag, or command that no longer
  exists. **Verify before editing** — check the referent on disk; do not assume from the name.
- **DELETE** — a task log (records what happened rather than a rule), a one-off conclusion, or
  something the code, git history, or CLAUDE.md already states. Narrative belongs in a
  changelog, not in memory.
- **MOVE** — wrong scope. A `type: user` fact in one project's dir can never be recalled from
  another; a project-specific gotcha in a shared dir is noise everywhere else. Note the hard
  limit: auto memory is keyed per repo, so **no auto memory is cross-project** — a fact that
  must apply everywhere belongs in `~/.claude/CLAUDE.md`, not in memory at all.
- **SPLIT** — a memory much longer than its neighbours is usually two facts in one file, or a
  rule with the incident narrative attached. Keep the rule, cut the narrative.

Also check the index itself: `MEMORY.md` must be one line per memory and nothing else. Content
parked there is charged to every session in that project.

## Step 3: Propose once, then apply

Present a single table — file, bucket, one-line reason — and get one approval for the batch.
Then apply, and re-run the Step 1 measurements to show the result.

**When in doubt, delete.** The costs are not symmetric: a missing memory only costs re-deriving
the fact, while a wrong one is asserted confidently on every match, and the model does not
notice on its own that it has gone stale. Bloat also degrades retrieval directly, not just
token count — near-duplicates compete with the entry you actually needed.

## Step 4: Stamp

```bash
date +%s > "$HOME/.claude/.memory-prune-stamp"
```

Without this the SessionStart reminder keeps firing, which is how a useful reminder turns into
noise that gets ignored.
