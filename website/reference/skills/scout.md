---
title: /znf:scout
---

# `/znf:scout [symbol | collection | endpoint | file:line being changed]`

Map what depends on something before you change it — the reverse question. Use before modifying existing code, an existing data shape, or anything shared: finds consumers, the tests that cover it, other systems written in the same operation, and why the code exists. Read-only and safe to run on its own. This is discovery; verifying that a name or shape is real is /ground's job, not this one.

## Cách gọi

```text
/znf:scout [symbol | collection | endpoint | file:line being changed]
```

## Tool được phép

`Read Grep Glob Bash(git *) Bash(rg *) Agent`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
