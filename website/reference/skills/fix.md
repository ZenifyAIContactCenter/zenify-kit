---
title: /znf:fix
---

# `/znf:fix [description | github-actions-url | empty for auto-log]`

Debug and fix a bug. Use when something is broken — auto-fetches logs/stacktrace to ground the diagnosis in real data. Pass a description, a GitHub Actions URL, or nothing (auto-detect recent logs).

## Cách gọi

```text
/znf:fix [description | github-actions-url | empty for auto-log]
```

## Tool được phép

`Read Grep Glob Bash Agent WebFetch`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
