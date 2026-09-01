---
name: contract-sweep
description: Sweep a changed contract across all repos to find producers/consumers and verify no drift. Use when changing a shared DB collection, API endpoint, or pub/sub event that other services depend on.
disable-model-invocation: true
allowed-tools: Bash(git *) Agent
---

**Explicit opt-in** — runs across multiple repos. Use after changing a shared contract.

## Not the default gate — the project's own gate is

Where a project defines a contract gate of its own (a project-local skill under its
`.claude/skills/`), that is the routine one: a single inline pass, cheap, run after every
shared-resource edit. This workflow is the escalation, not the habit. Reach for it only when one
inline pass cannot be trusted:

- too many usages to judge as a whole, so each needs its own BREAKING / RISKY / SAFE verdict
- the change spans several shared resources at once (a collection field *and* the queue
  payload carrying it)
- `/gate` came back clean but the change still feels wrong — a fresh agent per repo has no
  memory of the edit and so does not share the blind spot

This skill fans out one agent per repo and verifies every usage individually, which costs
real tokens; that is why it is `disable-model-invocation: true`.

Both skills use the same eight-repo list and the same three-pass search, so they should
never disagree. If they do, one of the two files has drifted — fix it rather than picking
whichever one you happened to open.

## How to run

```
Invoke Workflow tool with:
  scriptPath: "/Users/believe/.claude/workflows/contract-sweep.js"
  args: {
    contract: "e.g. 'chat_messages.content field' or '/api/v1/tickets endpoint'",
    changeDesc: "human description of what changed"
  }
```

Omit `repos` to sweep the full default set — only pass it to narrow the sweep, or for a
side project with a different layout.

The workflow:
1. Fans out one agent per repo to find all usages (producer/consumer/definition)
2. For each usage, verifies whether it will break (BREAKING / RISKY / SAFE)
3. Reports required co-changes before the change can be safely deployed

## Why the search is three passes

A collection-name grep finds the definition site and nothing else, because the literal
appears once per repo and every reader reaches the collection through a model symbol. The
workflow prompt therefore forces three passes: definition → model symbol usage → raw
driver. Pass 2 is where essentially all the volume is, and it is the one a naive sweep
skips, which is how a sweep can report "no usages" against hundreds of real call sites.

Three access styles, usually concentrated in different repos, so a sweep that knows only one
finds only that one: framework DI (`@InjectModel(Class)` and equivalents), a **model registry**
object where a single declaration file fans out to thousands of call sites, and the **raw
driver** (`.collection('x')`), which bypasses every model and every schema. Which repo uses which
is a project fact — read the workspace's `CLAUDE.md`. Aggregation-pipeline stage strings
(`$match`, `$project`, `$group`, `$lookup`) hide field usage from any static analysis.

## When to use

- After editing a shared MongoDB collection field
- After changing an HTTP endpoint path or response shape
- After renaming or restructuring a pub/sub event or queue payload
- `/gate` for complex multi-service changes (scale-up of `/gate`)

## The repo list is required, and is a project fact

There is **no default**, deliberately: a hardcoded list makes this workflow silently wrong in
every other workspace and silently stale in the one it was written for. The workspace's own
`CLAUDE.md` names the repos that share a store — read it there.

Derive it by evidence rather than habit when the project has not written it down:

1. `rg -l '<the shared DB name>' --glob '!node_modules'` across the workspace — every repo that
   talks to that store.
2. **Plus** the ones that break *without* touching it: a frontend consuming API responses shaped
   by those collections, a service consuming events derived from them.
3. **Minus** anything on a separate store, however similar its name.

A change-stream / CDC subscriber, where one exists, goes at the top: it reacts to every document
change, so a shape change reaches it before anything else.

`change-stream-subscriber` is the one that matters most and was missing from the earlier
default: it reacts to every document change, so a shape change reaches it first.

Exclude on purpose, and say you did: repos on a **separate** store however similar the name, and
apps genuinely isolated from the contract. An exclusion nobody stated is indistinguishable from
one nobody thought of.
