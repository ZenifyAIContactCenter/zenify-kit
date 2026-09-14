---
title: /znf:contract-sweep
---

# `/znf:contract-sweep`

Sweep a changed contract across all repos to find producers/consumers and verify no drift. Use when changing a shared DB collection, API endpoint, or pub/sub event that other services depend on.

## Cách gọi

```text
/znf:contract-sweep
```

::: warning Chỉ user gọi
Skill này đặt `disable-model-invocation: true`: agent không tự chạy, bạn phải gõ lệnh.
:::

## Tool được phép

`Bash(git *) Agent`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
