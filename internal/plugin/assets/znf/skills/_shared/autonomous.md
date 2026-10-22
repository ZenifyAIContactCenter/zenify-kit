# Autonomous mode

Applies when `ZNF_AUTONOMOUS=1`. It overrides every human gate in the znf skills; everything else in a skill still applies.

## Rules

- Never ask a person: no AskUserQuestion, no question in prose, no "shall I proceed?".
- Wherever a skill asks or waits for a person, decide yourself and record one line
  `Ruling: <decision> — <why> — <cost if wrong>` in the ledger (SDD `progress.md`) or, outside SDD, in the ship pack's `## Intent`.
- Stop only at the five points below, via `zenify await`.
- Pull requests: never merge one, including one you opened.

## Gate map

| Human gate in a skill | Autonomous replacement |
|---|---|
| `brainstorming` steps 2, 4, 7, 8 (shared understanding, clarifying rounds, per-section design approval, clarify-lite) | `Ruling` |
| `brainstorming` step 12 and the User Review Gate, plus `cook`'s design gate | `zenify await spec` (the design gate folds into the spec gate) |
| `cook` Step 5b plan approval | `zenify await plan` |
| `fix` Step 4, when the approach is a trade-off | `Ruling` |
| `ship` fix-loop still open after round 2 | `zenify await needs-human` |
| `_shared/data-scripts.md` prd gate before `--write` | `zenify await data-write` |
| SDD "Four things stop you": first three (irreversible/destructive, security-sensitive, side effect outside the worktree) | `zenify await irreversible` |
| SDD "Four things stop you": plan too broken to act on | `zenify await needs-human` |
| `discipline`: ask the user when routing a repo; `/cook` only suggested | `Ruling`; run `/cook` yourself when a task was assigned |

## Stopping

```
zenify await <spec|plan|data-write|irreversible|needs-human> --summary "<what the person must decide>" [--file <abs path>]… [--command "<command>"]
```

Pass every file the person must read as an absolute `--file`. Exit 8 is the expected stop, not a failure: after the command prints its id, end the session at once, with no further step and no retry.

`data-write`: dry-run first (`zenify data run --prd -- …`); `--command` carries exactly the `zenify data run --prd --write -- …` command you dry-ran. This is the worker's end point: approval means a lead runs that command; never resume to write. A `--stg --write` script does not stop: it runs right after its dry-run as `_shared/data-scripts.md` says.

## Resuming

The prompt is `resume <id> approved` or `resume <id> rejected: <reason>`. Run `zenify await show <id>` first, then:

- approved: continue from the stop point.
- rejected: fix what the reason names, then `zenify await` again at the same gate.
