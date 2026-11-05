# Architecture

## The public-distribution invariant

The `zenify` binary — and this repository's source — **must never embed
zenify-specific truth**: internal hostnames or IP addresses, database or
collection names, tenant IDs, employee data, internal repository lists, or any
secret or credential.

The binary is **pure mechanism**: workflow logic that is project-agnostic.
Everything specific to a particular workspace ("truth") is read at runtime from
that workspace's own private configuration — never compiled in.

### Why this matters

This repository and its release binaries are **public**. A compiled Go binary is
trivially inspected (`strings`, `objdump`), so anything embedded in it is
effectively published. Keeping the binary mechanism-only is exactly what makes
public distribution safe — the same model every commercial CLI uses: the client
is public, and all sensitive values are supplied at runtime and enforced
elsewhere.

### The rule for contributors

Before adding a subcommand or a `doctor` check: if it needs a workspace-specific
value, read it at runtime (an environment variable, a file in the workspace, a
flag) — do not hardcode it. A grep of this repository for internal IPs, database
names, or tenant IDs must always come back empty.

## Harness boundary

The kit runs inside an agent harness (Claude Code and Codex), and the boundary
between the two is deliberate:

- **The `zenify` binary is harness-agnostic.** It reads files, git state and
  transcripts on disk and prints text or JSON. It never calls a harness tool
  and never assumes one exists. A subcommand that would only make sense inside
  a particular harness belongs in the adapter layer below, not in the binary.
- **Hooks and skill frontmatter are the only adapter layer.** Hooks live in
  `<ws>/.claude/settings.local.json` and `<ws>/.codex/hooks.json`, both generated from the
  single `znfHookSpecs()` table (Codex gets the subset it can run, with its own
  subagent tool name) and forwarding to `zenify hooks-run <id>` or
  `zenify git-guard`. Skill frontmatter (`allowed-tools`, `context`,
  `background`, `disable-model-invocation`) tells the harness how to load a
  skill. Everything that must know a tool's *name* lives in one of these two
  places.
- **One skill tree, one agent source.** Skills exist once, under
  `~/.zenify/plugin-marketplace/plugins/znf` (plugin `znf@zenify-kit`, installed
  `--scope local` at the workspace root); Codex reaches them through a symlink at
  `<ws>/.agents/skills/znf`. Codex agents are generated from `agents/*.md` into
  `<ws>/.codex/agents`. The kit is inactive outside the workspace root.
- **Skills describe actions, not tools.** A skill body says "dispatch a
  subagent" or "open the task ledger"; the binding from action to tool name is
  the single table `skills/_shared/harness-tools.md`. Checkable named lines
  (`Skill(znf:run)`, an `Agent({...})` call shape) are the exception and stay
  literal, because the gates grep for them.

Why: Claude Code renamed `Task` → `Agent` once, and the kit absorbed it with a
`Task|Agent` matcher instead of a lesson. With the table, the next rename is
one file plus the hook matcher; without it, it is every skill.
