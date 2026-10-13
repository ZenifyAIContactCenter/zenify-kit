---
name: understand-codebase
description: Build a structural map of an unfamiliar codebase using parallel readers. Use when starting work in a new or unfamiliar repo — produces a system map, subsystem roster, and CLAUDE.md outline.
disable-model-invocation: true
allowed-tools: Read Glob Bash(ls *) Bash(find *) Bash(cat *) Agent
---

**Explicit opt-in** — runs multiple parallel readers. Use at project start, not mid-feature.

## How to run

1. **Scout it yourself** with `ls`/`find`/`cat`: stack, entry points, top-level subsystems. Pick at
   most 8 subsystems (the `focus` the user named comes first).
2. **One reader per subsystem, all in ONE message**: a read-only `Explore` agent, model `sonnet`, brief =
   the subsystem path + "what lives here, entry points, data it reads/writes, contracts it exposes,
   gotchas; ≤ 40 lines". Collect each report by name; a missing one leaves that subsystem marked
   "not read", never "nothing there".
3. **Synthesize** in the main loop: overview paragraph, subsystem roster, contract map, gotchas,
   CLAUDE.md outline.

## When to use

- Mapping a complex repo before writing or rewriting its `CLAUDE.md`
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
