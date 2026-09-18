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

The kit runs inside an agent harness (today: Claude Code), and the boundary
between the two is deliberate:

- **The `zenify` binary is harness-agnostic.** It reads files, git state and
  transcripts on disk and prints text or JSON. It never calls a harness tool
  and never assumes one exists. A subcommand that would only make sense inside
  a particular harness belongs in the adapter layer below, not in the binary.
- **Hooks and skill frontmatter are the only adapter layer.** `hooks.json`
  matches on harness tool names (`Agent|Task`, `Skill`, …) and forwards to
  `zenify observe …`; skill frontmatter (`allowed-tools`, `context`,
  `background`, `disable-model-invocation`) tells the harness how to load a
  skill. Everything that must know a tool's *name* lives in one of these two
  places.
- **Skills describe actions, not tools.** A skill body says "dispatch a
  subagent" or "open the task ledger"; the binding from action to tool name is
  the single table `skills/_shared/harness-tools.md`. Checkable named lines
  (`Skill(znf:run)`, an `Agent({...})` call shape) are the exception and stay
  literal, because the gates grep for them.

Why: Claude Code renamed `Task` → `Agent` once, and the kit absorbed it with a
`Task|Agent` matcher instead of a lesson. With the table, the next rename is
one file plus the hook matcher; without it, it is every skill.
