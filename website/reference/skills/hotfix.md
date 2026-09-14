---
title: /znf:hotfix
---

# `/znf:hotfix [short-kebab-desc]`

Handle a live production bug in a polyrepo workspace — diagnose first, then decide the response with the user (revert, disable, or fix forward), and only for a forward fix create an isolated worktree branched from the repo's configured hotfix base ref (never the default feature base). Scouts what depends on the code, verifies, gates and ships. Commits + pushes the hotfix branch once verified and opens the PR, but never merges. User-invoked only (whether something is a hotfix is the user's urgency call). If a bug looks live-critical, you may SUGGEST running /hotfix, but don't run it automatically.

## Cách gọi

```text
/znf:hotfix [short-kebab-desc]
```

::: warning Chỉ user gọi
Skill này đặt `disable-model-invocation: true`: agent không tự chạy, bạn phải gõ lệnh.
:::

## Tool được phép

`Bash(git *) Bash(wt *) Bash(zenify *) Read Grep Bash(rg *) Bash(cat *) Agent`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
