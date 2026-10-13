---
name: retro
description: Turn this session's costly moments into lessons, each filed in exactly one home — a check, a rule, a CLAUDE.md line, or memory — proposed as one list for the user to approve. Use when the user asks for a retro, or to capture what this session had to learn the hard way.
disable-model-invocation: true
allowed-tools: Read Grep Glob Bash Edit Write
---

**Propose, never decide.** Retro drafts lessons; the user chooses which ones land. It is
user-invoked only because every lesson it writes is loaded into future sessions.

## Step 1: Source

Read the conversation in context. On Claude Code, if the session was compacted, also read this
session's transcript (jsonl) under `~/.claude/projects/<slug>/` for the part compaction dropped.
On other harnesses, use the context only. Do not rely on an environment variable to find it: use the transcript file under
`~/.claude/projects/<slug>/` whose slug matches the current working directory, most recently modified.

## Step 2: Filter

Keep a moment only if one of two checkable things is true: you had to read, query, or run
something to learn it, or the user corrected you. "It felt non-obvious" is not a filter. Quote
each kept moment verbatim as evidence — the quotes stay in chat and never go into a file, and
secrets (tokens, passwords, connection strings) are redacted in the quotes too.

**Nothing qualifies →** say "no lessons" and stop. Write nothing.

## Step 3: One home per lesson

Each lesson gets one home, never two:

- **Machine-checkable → check.** Retro does not build it: write no file and change no code.
  Print a follow-up item in chat — which check, which repo, and whether it is `/cook` or `/fix`
  work — for the user to open.
- **Needs judgment → rule.**
- **Routing (where something lives, which repo or command) → one `CLAUDE.md` line.**
- **Measured gotcha → memory.**

Take each destination path from the project's and the harness's own config (for memory, the
harness's memory setting, else its default; for rules, where the project keeps
them). Never assume a path; say which file each item would land in.

## Step 4: Update, don't accumulate

Before proposing, read the existing memory and rules on the same subject. Overlap → propose an
edit to that one. Contradiction → propose a fix to it. Never propose a second authoritative entry
on one subject: the loser is whichever one happens to be read.

## Step 5: Approve once

Present one list — lesson, home, target file, one-line reason, and the verbatim quote behind it —
then ask the user to choose between options once for the whole list. Never ask per item: approval
habituation starts at the second prompt, so a per-item gate turns into reflexive clicking (the
same reasoning as `prune-memory`).

## Step 6: Write

Write only the items the user chose. Write the rule, not the event: "X reads Y from env, so the
file on disk is not what runs" changes the next decision; "fixed the bug on Tuesday" does not.
No transcript excerpt in any file, and strip secrets (tokens, passwords, connection strings) before
writing. Check items stay in chat. Finish by listing each file written.
