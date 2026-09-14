---
title: /znf:onboard-project
---

# `/znf:onboard-project [optional: what the new project should be]`

Bootstrap or map a project so the agent has an accurate system map before doing feature work. Use at the start of working in a project — when there is no CLAUDE.md yet, when joining an unfamiliar codebase, or when starting a brand-new project from scratch. The global harness (git-guard, Stop-hook memory, /ship evaluator, /run, superpowers) is already active for every project — onboarding wires the project-specific config (deploy branches, DB access, app recipe).

## Cách gọi

```text
/znf:onboard-project [optional: what the new project should be]
```

::: warning Chỉ user gọi
Skill này đặt `disable-model-invocation: true`: agent không tự chạy, bạn phải gõ lệnh.
:::

## Tool được phép

`Read Grep Glob Bash(ls *) Bash(cat *) Bash(find *) Bash(git *)`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
