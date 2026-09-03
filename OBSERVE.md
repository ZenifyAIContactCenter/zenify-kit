# zenify observe — Milestone 3 (Observe)

A single-binary, in-stack observability layer for Claude Code sessions. It counts
subagent dispatches, meters tool-output volume, summarizes sessions, and can render
a statusline HUD — all from the kit's own Go binary, driven by Claude Code hooks and
the statusLine stdin JSON. No external service, no Docker, no dashboard server.

## Why in-stack (not a fork of agents-observe)

`simple10/agents-observe` (MIT) is a real-time dashboard that observes via **Claude
Code hooks on stdin** — the same interface this kit already owns through
`zenify observe`. Its valuable capture layer is Claude Code's, not its own; the only
genuinely new part is a React/SQLite/Docker dashboard, which is off the kit's
single-Go-binary ethos. So M3 **harvests** the idea into `zenify observe report`
rather than forking. If a full web dashboard is ever needed (replay, filtering,
token graphs), run agents-observe **alongside** — it registers its own hooks and
coexists with the znf hooks.

## The four pieces

| # | Piece | Mechanism | UI-independent? |
|---|---|---|---|
| count | dispatch counter + soft-cap warn | PreToolUse hook (`Task`) | yes — model-facing `additionalContext` |
| meter | per-session tool-output accounting | PostToolUse hook (`Task\|Bash\|WebFetch\|WebSearch\|Read`) | yes — passive data |
| report | per-session summary (table / `--json`) | human-invoked CLI | CLI only |
| statusline | one-line HUD | settings.json `statusLine` | **CLI/TUI only** — not rendered in the VS Code / JetBrains extensions |

Hooks (count, meter) fire in **both** the CLI and the VS Code extension. The
statusline is the only piece the VS Code extension does not render.

## State layout

Everything lands under `$XDG_STATE_HOME/zenify/observe/` (falling back to
`~/.local/state/zenify/observe/`), one directory per sanitized `session_id`:

```
<session>/count.json    {"count": N}                                    # dispatches
<session>/meter.json    {"calls":{"Tool":N}, "bytes":{"Tool":N}}        # tool-output
```

All writes take a per-session `flock`; all reads are lock-free snapshots. Every hook
path is fail-open and exits 0 — observability never blocks a tool call.

## Install

The hooks live in `hooks.json`, which is **embedded in the binary** via `go:embed`.
So a hook change is only reflected after the binary is rebuilt **and** re-synced:

```bash
# 1. rebuild + reinstall the kit binary (so the embedded hooks.json is current)
cd zenify-kit && go build -o "$(command -v zenify)" ./cmd/zenify   # or your install path

# 2. materialize the embedded plugin into ~/.claude (additive, refresh-safe)
zenify skills sync

# 3. hooks are now registered. count + meter run automatically on the next session.
```

`observe count` and `observe meter` are **hidden hook subcommands** — you never call
them by hand; Claude Code invokes them per tool call. `observe report` and
`observe statusline` are user-facing.

## Usage

### report — the in-stack dashboard

```bash
zenify observe report          # table: SESSION  DISPATCH  CALLS  TOOL-OUT  LAST-ACTIVE
zenify observe report --json   # machine-readable, newest-active first
```

### statusline — opt-in HUD (CLI/TUI only)

**Full line** (for a fresh setup with no statusline). This is a settings.json key —
a plugin cannot ship one, and only ONE statusLine slot exists, so this **replaces**
any existing statusline:

```json
"statusLine": { "type": "command", "command": "zenify observe statusline" }
```

Renders: `model · ctx% · ⟳dispatches · ↓tool-output/calls · $cost`.

**Segment splice** (when you already have a statusline you like). `--segment` prints
**only** the two kit-unique segments (`⟳dispatches · ↓tool-output`) and drops
model/ctx/cost, so it composes into an existing line instead of overwriting it. Pipe
your statusline's same stdin JSON to it and append the output. Example, for a bash
statusline that captured stdin into `$input` and is building `$line2`:

```bash
if command -v zenify >/dev/null 2>&1; then
  znf_seg=$(printf '%s' "$input" | zenify observe statusline --segment 2>/dev/null)
  [ -n "$znf_seg" ] && line2+="  |  $znf_seg"
fi
```

The guard is deliberate: an older `zenify` without the `observe` command errors to
stderr and prints nothing, so the splice stays inert until the binary gains the
flag, then lights up on its own. For a statusline that is a closed binary (e.g.
claude-hud) rather than a script you own, there is no way to auto-splice — configure
it on that tool's side or switch to the full line.

## Soft cap (count)

`observe count` warns — never blocks — when a session's `Task` dispatches exceed the
cap resolved from the environment (`ResolveCap`). The warning is delivered to the
model as `hookSpecificOutput.additionalContext`, so it is visible regardless of which
client (CLI or extension) is in use. Cap ≤ 0 disables it.

## Branch stack (pre-merge state)

M3 shipped as four branches. Three are stacked; skill-brevity is independent:

```
main
 └─ observe-metering   #3  PostToolUse meter hook + hooks.json PostToolUse entry
     └─ hud-statusline #5  statusline HUD  (+ statusline --segment)
         └─ observe-report #1  report subcommand + this doc

skill-brevity          #4  safe dedup of duplicated skill mechanism text (independent, base main)
```

Merge order: **#3 → #5 → #1** (stacked, so in that order), **#4** any time. Once all
land on `main`, `main` carries every piece — including `statusline --segment`, which
currently lives only on the `hud-statusline` branch.

## Design notes captured in memory

- `posttooluse-hook-append-only` — a PostToolUse hook cannot modify/truncate the tool
  output the model sees (only additive `systemMessage`/`additionalContext`), so #3 is
  **metering-only**; output compression was dropped and belongs to built-in tool
  limits (`BASH_MAX_OUTPUT_LENGTH`, etc.).
- `agents-observe-harvest-decision` — why harvest, not fork.
- `znf-skill-brevity-audit` — the #4 dedup scope (safe dedup only, narrative kept).
