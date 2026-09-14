---
title: /znf:review
---

# `/znf:review`

The kit's unified review engine. Mechanically selects a tier based on the diff, dispatches reviewers (T1 solo / T2 fan-out / T3 adversarial), and returns findings per the shared schema. The main gate for /review and for ship step 5.

## Cách gọi

```text
/znf:review
```

## Tool được phép

`Bash(git *) Bash(rg *) Bash(bash *) Bash(test *) Bash(awk *) Bash(zenify *) Agent Workflow`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
