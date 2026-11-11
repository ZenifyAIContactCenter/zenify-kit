---
name: track
description: Manage the track ledger by short code — add or drop a backlog item, list Plan and Backlog, plan the day, start, finish or cancel an item, set its estimate. Use when the user says "track", "backlog", "plan today", "next task", or types /znf:track.
argument-hint: "help | add <desc> [--plane] | drop <code> | ls | plan [+code|-code…] | go <code> | done [--pr <url>] | cancel | est S|M|L"
allowed-tools: Read Grep Glob Bash(zenify track*) Bash(zenify plane create*) Skill
---

**One skill, many verbs.** The first word of the args is the verb. Every write goes through the
`zenify track …` CLI; this skill never edits `.zenify/track/` itself. Commands and recording
rules: `znf:_shared/track.md`.

## Step 1: Pick the verb

Rule: no verb or an unknown verb → print help, run no write command. For an unknown verb print
the help table below preceded by the line `unknown verb: <verb>`. `help` prints the table.
`help <verb>` prints only that verb's row plus the `zenify track …` command it runs.

## Help table (print verbatim)

| Verb | What it does | Example |
|---|---|---|
| `add <desc> [--plane]` | add a backlog item; prints its code | `/znf:track add fix the export timeout` |
| `drop <code>` | remove an item from the backlog | `/znf:track drop b81e` |
| `ls` | print Plan, then Backlog, one line per item with its code | `/znf:track ls` |
| `plan` | build today's plan; asks for approval first | `/znf:track plan` |
| `plan +<code>…` | add items to today's plan | `/znf:track plan +b81e +#581` |
| `plan -<code>…` | remove items from today's plan | `/znf:track plan -b81e` |
| `go <code>` | start the item | `/znf:track go ZEN-288` |
| `done [--pr <url>…]` | finish the open item | `/znf:track done --pr https://github.com/org/repo/pull/9` |
| `cancel` | cancel the open item | `/znf:track cancel` |
| `est S\|M\|L` | set the open item's estimate | `/znf:track est M` |
| `help [verb]` | print this table, or one verb | `/znf:track help go` |

Codes: `b81e` = backlog id suffix · `#581` = issue number · `<repo>#581` = issue in that repo
(use when `#581` is ambiguous) · `ZEN-288` = Plane item key.

## Step 2: Run the verb

Pass the code to the CLI as typed; the CLI resolves it. Never resolve a code yourself.

- `add <desc>`: infer repo, est and ref from the conversation; repo unclear → ask. Run
  `zenify track backlog add "<title>" --repo <repo> [--est S|M|L] [--ref <url|memory:<name>>]`;
  print the code of the new item. With `--plane`: requires `zenify plane create`, available after PR2;
  until then say it is not available, add nothing.
- `drop <code>`: `zenify track backlog drop <code>`.
- `ls`: `zenify track stats --brief`; print Plan, then Backlog, each line with its `code`.
- `plan`: the Daily plan procedure in `znf:_shared/track.md`; run `plan set` only after the user
  approves.
- `plan +<code>…` / `plan -<code>…`: `zenify track plan add <code>…` / `zenify track plan rm <code>…`.
- `go <code>`: a code task invokes `Skill(znf:cook)` or `Skill(znf:fix)` (by the nature of the
  work; ask when unclear) and tells it to pass `--from <id>` on its `start`. Ops work, no PR:
  `zenify track start --kind task --repo <repo> --est <S|M|L|-> --from <code>`; keep the printed id.
- `done [--pr <url>…]`: no `--pr` → `zenify track end <id> --done --rounds 0`; with `--pr` →
  `zenify track end <id> --pr <url> [--pr <url>…] --rounds 0`.
- `cancel`: `zenify track end <id> --cancelled --rounds 0`.
- `est S|M|L`: `zenify track est <id> <S|M|L>`.

## Open id (done, cancel, est)

`<id>` is the track id this session got from the `track: <id>` line at `start`. Not in the
conversation any more → run `zenify track stats --brief` and read its `running` items:

- Exactly one `running` item → use it.
- two or more `running` items → ask the user which one.
- no `running` item → tell the user; never guess an id.

## CLI errors

A CLI exit that is non-zero (ambiguous or unknown code, `taken by`, `track: off`) → show the
message verbatim, with any candidate list, and let the user choose. Never retry on your own and
never switch to another code. `log is locked` → wait a few seconds and retry once.
