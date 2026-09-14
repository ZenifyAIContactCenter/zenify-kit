---
title: /znf:gate
---

# `/znf:gate [collection | channel | endpoint | queue name]`

Cross-repo contract gate for a polyrepo workspace. Run this automatically right after editing anything shared across services — a shared DB collection/table, a pub/sub channel, an HTTP endpoint between services, or a queue — to find every producer/consumer and verify nothing breaks. Read-only and safe to run on its own. Can also be invoked manually with a resource name.

## Cách gọi

```text
/znf:gate [collection | channel | endpoint | queue name]
```

## Tool được phép

`Grep Read Bash(grep *) Bash(rg *) Bash(zenify *) Agent`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
