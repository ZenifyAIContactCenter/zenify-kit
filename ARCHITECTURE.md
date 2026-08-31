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
