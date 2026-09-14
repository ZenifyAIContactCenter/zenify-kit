---
title: /znf:cook
---

# `/znf:cook <feature description or path/to/plan.md>`

Full feature pipeline in one command. Use when implementing a feature end-to-end — branch, brainstorm to a spec, ground every name against real data, plan, subagent-driven implementation, then the pre-ship gate. Always spec-driven and always subagent-driven, at every size. Commits and pushes the feature branch on all-green (house rule #7); never opens the PR.

## Cách gọi

```text
/znf:cook <feature description or path/to/plan.md>
```

## Tool được phép

`Read Grep Glob Bash Agent`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.
