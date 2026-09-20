# Harness tools — shared reference

**Last updated:** 2026-09-18

The kit's skills describe work at the **action** level ("dispatch a subagent", "open the task
ledger") and this table is the single place that binds each action to a concrete harness tool.
When a harness renames a tool (Claude Code renamed `Task` → `Agent` once already), this file
changes and the skills do not.

Two rules for skill authors:

- **Name the action in prose; name the tool only where a line must be checkable** — a
  `Skill(znf:run)` line in a transcript is auditable, "I ran the app" is not. Keep those named
  lines exactly as the skill spells them; they are what the gates grep for.
- **Never invent a tool name.** If an action is missing here, add the row, then use it.

## Action → tool

| Action | Claude Code | Other harness |
|---|---|---|
| Dispatch a subagent (fresh context, returns a report) | `Agent` (`subagent_type`, `model`, `prompt`) | — |
| Invoke another kit skill | `Skill` (`znf:<name>`) | — |
| Ledger of dispatched agents (one line each) | a file the skill names (SDD `progress.md`, ship board) — not `TodoWrite`/`TaskCreate`: each call is an extra API turn | — |
| Ask the user to choose between options | `AskUserQuestion` | — |
| Read a file / search text / find files | `Read` / `Grep` / `Glob` | — |
| Edit or create a file | `Edit` / `Write` | — |
| Run a shell command | `Bash` | — |
| Fetch a page or search the web | `WebFetch` / `WebSearch` | — |
| Drive a browser (UI verification) | `mcp__playwright__browser_*` via `znf:ui-verifier` | — |
| Wait on a condition or a background process | `Monitor` | — |

## Boundary

The `zenify` binary never calls any of these: it is harness-agnostic. The only adapter layer is
hooks (`hooks.json`, matched on tool names such as `Agent|Task`) plus skill frontmatter
(`allowed-tools`, `context`, `background`, `disable-model-invocation`). See `ARCHITECTURE.md`
"Harness boundary".
