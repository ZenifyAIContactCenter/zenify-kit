---
title: /znf:ground
---

# `/znf:ground`

Verify real shapes and real values before writing code. Use when about to write code that touches a DB field, API payload, queue message, external library API, in-repo function/symbol/component props, a config key or an env var — fetch the ACTUAL shape from its real source first, including which values a field really holds and which filters every query must carry. Answers "what is X?" only; for "what depends on X?" use /scout.

## Cách gọi

```text
/znf:ground
```

## Tool được phép

`Read Grep Glob Bash(zenify db-read *) Bash(mongosh *) Bash(mysql *) Bash(psql *) Bash(grep *) Bash(find *) Agent`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
