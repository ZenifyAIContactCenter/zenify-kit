---
title: /znf:understand-codebase
---

# `/znf:understand-codebase`

Build a structural map of an unfamiliar codebase using parallel readers. Use when starting work in a new or unfamiliar repo — produces a system map, subsystem roster, and CLAUDE.md outline.

## Cách gọi

```text
/znf:understand-codebase
```

::: warning Chỉ user gọi
Skill này đặt `disable-model-invocation: true`: agent không tự chạy, bạn phải gõ lệnh.
:::

## Tool được phép

`Read Glob Bash(ls *) Bash(find *) Bash(cat *) Agent`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
