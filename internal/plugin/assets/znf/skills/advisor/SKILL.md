---
name: advisor
description: "Second opinion from the strong model on ANY question, in a fresh context — the manual lever that replaces switching the session model. Use when the user types /znf:advisor, or when their message contains one of these exact phrases: \"hỏi fable\", \"ý kiến thứ hai\", \"second opinion\", \"@fable\", \"ultrathink\", \"nghĩ kỹ hơn\". Never invoke because a task merely feels hard. Unlike the harness's built-in advisor it reads only the brief and paths you give it, not the transcript, and every call is recorded in route-log." # <!-- znf:allow-lang -->
argument-hint: "<question> | <path>... <question>"
allowed-tools: Read Write Bash(mkdir *) Bash(date *) Bash(bash *) Bash(jq *) Bash(zenify route-log *) Bash(git *) Agent
---

**Announce:** "Using znf:advisor — one fresh-context consult, model chosen by select-route."

This skill runs inline in the main session and dispatches one agent. Its own frontmatter pins no
model — the choice comes from `select-route`, which reads the machine's `ZNF_STRONG_MODEL` and
returns `opus` where no stronger tier is configured. Nothing here detects access or prints a warning.

## Step 1 — trigger

`TRIGGER=manual` when the user invoked `/znf:advisor`; otherwise the exact phrase from the
user's message that matched the description list. No phrase and no slash command → do not run
this skill (say so in one line if you were about to).

## Step 2 — brief on disk

```bash
SLUG=$(date -u +%Y%m%dT%H%M%SZ); DIR=${TMPDIR:-/tmp}/znf-advisor-$SLUG; mkdir -p "$DIR"
```

Write `$DIR/brief.md`:

```
# Consult brief
trigger: <TRIGGER>
question: <the user's question, verbatim>
paths: <absolute paths the user named, one per line, or none>
answer contract:
- conclusion first, then confidence: Strong | Worth exploring | Speculative
- evidence you actually read: file:line, or URL when you delegated research
- what would change the conclusion
- what you could not check
- at most 40 lines; read only, change nothing
```

Long inputs (a PDF, a design doc) go in `paths`; the main session does not read them.

## Step 3 — route and dispatch

```bash
ROUTE=$(bash ~/.claude/skills/znf/skills/_shared/scripts/select-route manual)
MODEL=$(printf '%s\n' "$ROUTE" | sed -n '1s/^model=//p')
```

`Agent(general-purpose)` with `model` = `$MODEL` verbatim and the prompt:
"Read the brief at `$DIR/brief.md` and every path it lists. Answer per its contract. Do not
edit any file. Return at most 40 lines." Print `$ROUTE` line 2 on the report. Relay the answer
to the user as it came back.

## Step 4 — record (best-effort, never blocks)

```bash
command -v zenify >/dev/null && command -v jq >/dev/null && jq -nc \
  --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg repo "$(basename "$(git rev-parse --show-toplevel 2>/dev/null)")" \
  --arg branch "$(git branch --show-current 2>/dev/null)" --arg model "$MODEL" --arg strong "${ZNF_STRONG_MODEL:-opus}" --arg trig "$TRIGGER" \
  '{"ts":$ts,"repo":$repo,"branch":$branch,"site":"manual","features":{},"gates":["TAG"],"model":$model,"strong":$strong,"trigger":$trig}' \
  | zenify route-log record 2>/dev/null || true
```

Not this skill's job: designing a feature (that is `znf:brainstorming`'s architect step),
reviewing code (`znf:review`), fixing anything.
