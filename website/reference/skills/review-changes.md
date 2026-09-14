---
title: /znf:review-changes
---

# `/znf:review-changes`

Multi-dimensional code review with adversarial verification. Use when you want a thorough review of a diff before shipping — fans out across bug/security/perf/contract/type dimensions, then adversarially verifies each finding with 3 independent skeptics.

## Cách gọi

```text
/znf:review-changes
```

::: warning Chỉ user gọi
Skill này đặt `disable-model-invocation: true`: agent không tự chạy, bạn phải gõ lệnh.
:::

## Tool được phép

`Bash(git *) Agent`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
