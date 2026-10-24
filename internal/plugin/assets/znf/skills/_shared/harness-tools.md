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

| Action | Claude Code | Codex |
|---|---|---|
| Dispatch a subagent (fresh context, returns a report) | `Agent` (`subagent_type`, `model`, `prompt`) | `collaborationspawn_agent` with the same-named agent |
| Invoke another kit skill | `Skill` (`znf:<name>`) | `$znf:<name>` |
| Ledger of dispatched agents (one line each) | a file the skill names (SDD `progress.md`, ship board) — not `TodoWrite`/`TaskCreate`: each call is an extra API turn | same file |
| Ask the user to choose between options | `AskUserQuestion` | ask in plain text |
| Read a file / search text / find files | `Read` / `Grep` / `Glob` | `cat` / `sed -n` / `rg` / `find` |
| Edit or create a file | `Edit` / `Write` | `apply_patch` |
| Run a shell command | `Bash` | shell |
| Fetch a page or search the web | `WebFetch` / `WebSearch` | Codex web search |
| Drive a browser (UI verification) | `Bash: zenify browser -s=<session> …` via `znf:ui-verifier` | `Bash: zenify browser -s=<session> …` |
| Wait on a condition or a background process | `Monitor` | poll with a shell loop |
| Run a command in the background and be woken when it exits (waits longer than a `Monitor` deadline, e.g. CI) | `Bash` with `run_in_background: true` | run it in the foreground (blocks the turn) |
| Run a data script against stg or prd with broker credentials | `Bash`: `zenify data run (--stg\|--prd) [--write] -- <cmd>`, a bare call (no pipe, redirect or `cd`) — see `data-scripts.md` | same command |
| Query logs, traces, metrics or product analytics | MCP `mcp__<broker-grafana>__*`, `mcp__<broker-posthog>__exec` — see `evidence-sources.md` | same MCP servers |

Codex notes: `zenify browser` runs inside Codex's shell sandbox and is not verified there; Codex in this workflow does CI review only. `Workflow` has no equivalent (only `contract-sweep` uses it); review T3 is a plain parallel `Agent` fan-out and runs the same on Codex. `context: fork` frontmatter is
ignored, the skill runs inline. Codex passes the text after `$znf:<name>` as plain prompt text.

## Boundary

The `zenify` binary never calls any of these: it is harness-agnostic. The only adapter layer is
hooks (`hooks.json`, matched on tool names such as `Agent|Task`) plus skill frontmatter
(`allowed-tools`, `context`, `background`, `disable-model-invocation`). See `ARCHITECTURE.md`
"Harness boundary".
