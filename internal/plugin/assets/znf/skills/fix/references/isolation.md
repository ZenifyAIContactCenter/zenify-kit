<!-- Moved verbatim from fix/SKILL.md § Isolation (route-log slim). Read when: about to create the worktree at Step 5. -->

**Isolation (house rule #8): a worktree, always — whatever the size of the fix.**

> **Isolation & base-ref doctrine → znf:discipline §8** (single source): worktree unconditional, base = the declared baseRef, fetch before resolving it. Below is `/fix`'s operational step only.

```bash
git -C <repo> fetch origin                                   # before resolving the base — see znf:discipline §8
cd <repo> && wt new <slug> --type fix --base "$(node -e 'console.log(JSON.parse(require("fs").readFileSync(".claude/worktree.json","utf8")).baseRef)')"
```

One worktree per affected repo, same slug. No workspace handoff here, unlike `/cook`;
`Skill(znf:run)` still gives the repo a `dev` pane when the fix needs it running.

**A bug found mid-task is not a new task** — `cd` into the worktree that exists. Only unrelated
work earns `--another`; a live production bug is `/hotfix`. Do this **before Step 5**, not Step 0.
