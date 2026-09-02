---
name: gate
description: Cross-repo contract gate for a polyrepo workspace. Run this automatically right after editing anything shared across services — a shared DB collection/table, a pub/sub channel, an HTTP endpoint between services, or a queue — to find every producer/consumer and verify nothing breaks. Read-only and safe to run on its own. Can also be invoked manually with a resource name.
argument-hint: "[collection | channel | endpoint | queue name]"
allowed-tools: Grep Read Bash(grep *) Bash(rg *) Bash(zenify *) Agent
---

Verify the shared resource: **$ARGUMENTS** (if no argument was passed, verify the shared resource you just edited).

## 1. Find every usage across the polyrepo

**Never guess which repos "look relevant."** Run the participants command to get the real,
current set — a repo silently added or dropped from the shared store shows up here without
anyone updating this file:

```bash
zenify gate participants --json
```

This lists every repo in the workspace whose `.claude/worktree.json` declares
`gate.sharedStore=true`, each with `accessPatterns` (the kinds of access that repo uses to
reach the shared store — DI injection, a model/registry symbol, the raw driver, whatever
that repo's own config says) and `dbAccessor` (the read-only tool/command for querying the
real store from that repo, if one is configured). Sweep against **this output**, not against
any list written into this file — a repo that reacts to *every* document change (a
change-stream/CDC subscriber, if the workspace has one) is exactly the kind of participant
that goes missing from a hand-maintained list and breaks first when it does.

**Grepping `$ARGUMENTS` alone finds the definition site and nothing else.** The resource
literal appears once per repo; every reader reaches it through a model/registry symbol. Do
three passes per repo, using that repo's `accessPatterns` to know which of these apply:

1. **Definition** — where the resource name is declared (a schema/collection declaration, a
   route registration, a queue name constant). If no explicit name is declared, the
   framework may derive one from a class/module name — check before assuming a 1:1 name match.
2. **Model/registry-symbol usage — this is where all the volume is, and the pass usually
   skipped:** whatever access pattern(s) that repo's `accessPatterns` name — e.g. a DI
   injection pattern (`InjectModel(Class)`-style), a central registry object that fans out to
   many call sites, or the class/variable the resource resolves to at the call site.
3. **Raw driver and pipelines** — direct driver calls that bypass the model/registry layer
   entirely, plus the bare field name inside aggregation/pipeline stage strings, which no
   static analysis can see.

Do not report "no usages" on the strength of pass 1.

### Run the sweeps concurrently — one `Explore` agent per participant repo

This is the slowest step in every pipeline that calls it, and it parallelises perfectly: the
participant repos are independent, so nothing found in one changes how another is searched.

**Dispatch all participants in a SINGLE message, each with `model: 'sonnet'`.** The single
message is what makes them concurrent — one message each runs them in sequence and buys
nothing but the same wait. The model must be named: `Explore` pins none, so an omitted
`model` inherits the session's, and running N agents through three `rg` passes at the top
tier is overkill. The work is pattern matching against fixed patterns given to them —
sonnet's case exactly.

Give each agent exactly one repo path, the resource name, that repo's `accessPatterns`, **all
three passes above verbatim**, and this output contract:

```
Repo: <path>
Definition:      <file:line, or "none">
Model symbol:    <the Class / registry key it resolved to, or "none">
Readers/writers: <file:line per hit — path and line only, no code dumps>
Pipeline stages: <file:line where the field appears inside pipeline/aggregation stages>
Cannot enumerate by grep: <anything dynamic, or "none">
```

Why agents rather than inline: the intermediate volume here can be enormous — a registry
pattern in one repo can fan out to thousands of call sites — and once you have the
`file:line` list, the match dumps are worthless. Output that is a **map** delegates cleanly;
output that is **evidence** does not, which is why step 3 below stays inline.

**Two things that must survive the fan-out:**

- **Never let an agent decide its own repo is irrelevant.** Each is told its repo and returns
  a result for it, including `none`. The participant set comes from `zenify gate
  participants`, not from judgement — a change-stream-style subscriber (if the workspace has
  one) reacts to every document change and breaks first.
- **Ask each participant for its report by name.** With several agents out at once, a lost
  report reads exactly like a repo with no hits, and only one of those is safe to act on
  (house rule #3). A missing report means this gate is **incomplete**, not clean — say so
  rather than reporting N repos checked.

## 2. Analyze by resource type
- **DB collection/table**: for each hit, note which fields it reads/writes. Does your change break any reader?
- **Pub/sub channel**: find the publisher and the subscriber; compare the payload shape on both sides.
- **HTTP endpoint**: find the route definition (server) and every caller (possibly another repo); compare request/response shape.
- **Queue**: find the producer (add job) and consumer (worker); compare job payload.

## 3. Verify names and fields against REAL data (anti-fabrication — required for DB resources)

Do **not** trust a collection/table name or a field name from code or docs. Both have been
wrong in this kind of sweep before.

**Resolve the real name first — never type one from memory.** A schema's declared name and
the store's actual collection/table name can differ (an ORM may pluralise, or a project may
declare one thing and use another silently). Some stores create a collection/table silently
on first write, so a wrong name can return zero rows with no error, and a write can leave a
new empty one behind.

Use the `dbAccessor` reported by `zenify gate participants --json` for the repo in question —
that is the workspace's own read-only tool/command for checking real data, if it configured
one. Where a repo has none configured, say so rather than guessing at a query tool.

> ⚠️ **A read-only accessor is a guard against accidents, not a barrier** if the underlying
> connection can write. **Never issue a write query directly** through it — a write goes in a
> purpose-written script that connects and runs, so the intent is reviewable before it
> executes.

## Output

### Usages found
| Repo | File:line | Role (reader/writer/publisher/subscriber/caller) | Notes |
|---|---|---|---|

### Assessment
- **Breaking?** yes/no — why (cite `file:line`)
- **Real-data check:** what the actual document/row/shape showed
- **Also needs changing:** files, if any
- **Deploy order:** subscriber before publisher for breaking pub/sub changes

---

## This is the default gate. When to escalate

`/gate` is what runs after every shared-resource edit: one inline pass, cheap, read-only,
safe to run unprompted. Use it by default and do not ask permission first.

Escalate to the global **`contract-sweep`** skill only when a single inline pass is not
enough to trust the answer:

- the sweep turns up more usages than you can hold in your head at once, so each one needs
  its own independent BREAKING / RISKY / SAFE verdict rather than one overall judgement
- the change spans several shared resources at the same time (a collection field *and* the
  queue payload that carries it, say)
- `/gate` came back clean but the change still feels wrong — a fresh agent per repo, with
  no memory of the edit, is not subject to the same blind spot

`contract-sweep` fans out one agent per repo and then verifies every usage individually, so
it costs real tokens and is `disable-model-invocation: true` — it must be invoked by hand.
Both skills draw the participant set from `zenify gate participants` and use the same
three-pass search, so they should never disagree; if they ever do, one of the two files has
drifted and needs fixing.
