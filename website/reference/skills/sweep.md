---
title: /znf:sweep
---

# `/znf:sweep`

Tear down a finished task — stop its dev servers, close its terminal workspaces (when a workspace manager is present), remove its worktrees and branches, across every repo it touched. Use when work has landed and the workspace should go back to clean, or when asked to clean up or tidy up after a task. Refuses to report success when nothing has actually merged yet, and says what is still needed instead.

## Cách gọi

```text
/znf:sweep
```

## Tool được phép

`Bash(wt *) Bash(git *) Bash(node *) Bash(ls *) Read`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
