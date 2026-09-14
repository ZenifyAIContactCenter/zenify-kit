---
title: /znf:run
---

# `/znf:run`

Launch the app and produce real output from the real code path, so a change can be verified rather than asserted. Use when a change is behavioural and there are no tests covering it, before claiming it works, and before dispatching znf:ui-verifier (which needs the URL this produces). Reads the port the worktree was allocated instead of hunting for a free one.

## Cách gọi

```text
/znf:run
```

## Tool được phép

`Read Grep Glob Bash(git *) Bash(rg *) Bash(cat *) Bash(nc *) Bash(curl *) Bash(node *) Bash(tail *) Bash(grep *)`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
