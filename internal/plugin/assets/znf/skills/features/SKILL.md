---
name: features
description: List or toggle kit features (such as the dev track pane) from inside Claude Code through `zenify feature`. Use when the user says "enable track", "turn track on or off", "list kit features", or runs /znf:features.
allowed-tools: Bash(zenify feature*)
---

**Toggle through `zenify feature`, never act beyond it.** This skill edits no file itself, runs no
other command, and dispatches no agent. `zenify feature` writes the workspace overlay.

## Step 1: Parse the request

- No argument → list: run `zenify feature`.
- `<key> on|off` (for example `track on`) → run `zenify feature <key> on|off`, then run
  `zenify feature` to show the resulting state.
- Anything else (a missing value, a value other than `on` or `off`) → ask which feature and which
  value, listing the keys from `zenify feature`. Do not guess.

## Step 2: Report

- Print the command output verbatim. Do not paraphrase, reorder, or summarise it.
- A command that fails (unknown key, bad value, not in a workspace) → print its error verbatim and
  stop. Do not retry with another key or value, do not edit the overlay by hand.
- After enabling `track`: say the status line updates by itself within a few seconds (the pane
  re-reads the overlay). If the workspace is not onboarded to track yet, the next session's digest
  will remind the user.
