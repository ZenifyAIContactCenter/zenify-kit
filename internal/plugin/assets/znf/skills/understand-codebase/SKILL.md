---
name: understand-codebase
description: Build a structural map of an unfamiliar codebase using parallel readers. Use when starting work in a new or unfamiliar repo — produces a system map, subsystem roster, and CLAUDE.md outline.
disable-model-invocation: true
allowed-tools: Read Glob Bash(ls *) Bash(find *) Bash(cat *) Agent
---

**Explicit opt-in** — runs multiple parallel readers. Use at project start, not mid-feature.

## How to run

```
Invoke Workflow tool with:
  scriptPath: "/Users/believe/.claude/workflows/understand-codebase.js"
  args: {
    repoPath: "/path/to/repo",
    focus: "optional: what you want to understand, e.g. 'authentication flow' or 'billing'"
  }
```

The workflow:
1. Scouts top-level structure, stack, entry points, subsystems
2. Parallel reads of each subsystem (up to 8 concurrently)
3. Synthesizes into: overview paragraph, subsystem roster, contract map, gotchas, CLAUDE.md outline

## When to use

- `/onboard-project` MAP mode for complex repos (delegate from onboard)
- Starting on a new side-project repo you haven't worked in before
- Building `CLAUDE.md` from scratch for a repo with no documentation

## Output used for

The narrative output becomes the basis for:
- `CLAUDE.md` for the project
- Context for `/ground` to know which collections exist before querying one
- Contract map for `contract-sweep`

**This map is an index, not ground truth — it does not replace `/ground`.** It is a synthesis
of subagent summaries: it tells you *where* things live and *what exists*, never the exact
shape of anything. Use it to know what to go and verify.
