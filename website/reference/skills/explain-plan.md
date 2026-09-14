---
title: /znf:explain-plan
---

# `/znf:explain-plan`

Use when a diff adds or changes a DB query — the mandatory two-tier DB-perf gate. Runs a static scan (no DB needed) plus a per-query explain plan, and classifies findings BLOCKING (surface as must-fix at ship) vs ADVISORY. Degrades cleanly when the DB is unreachable.

## Cách gọi

```text
/znf:explain-plan
```

## Tool được phép

`Read Grep Bash(zenify db-read *) Bash(zenify db-perf *) Bash(git diff *)`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
