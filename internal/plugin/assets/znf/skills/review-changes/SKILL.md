---
name: review-changes
description: Multi-dimensional code review with adversarial verification. Use when you want a thorough review of a diff before shipping — fans out across bug/security/perf/contract/type dimensions, then adversarially verifies each finding with 3 independent skeptics.
disable-model-invocation: true
allowed-tools: Bash(git *) Agent
---

> **Vai trò:** đây là T3 backend nội bộ của `znf:review`. Người dùng nên vào `/review` (engine tự chọn tier); chỉ tier T3 mới chạy workflow này.

**Explicit opt-in only** — this workflow is token-heavy. Use for diffs larger than ~200 lines or when correctness is critical.

## How to run

1. Get the diff:
```bash
git diff HEAD   # or git diff <base>..<head>
```

2. Run the `review-changes` workflow:
```
Invoke Workflow tool with:
  scriptPath: "~/.claude/skills/znf/workflows/review-changes.js"
  args: {
    diff: <the git diff output>,
    context: "<optional: task intent, relevant contracts, API shapes>"
  }
```

3. The workflow fans out across 5 dimensions (bugs/security/perf/contracts/types), then adversarially verifies each finding with 3 skeptics — only findings confirmed by ≥2 skeptics survive.

4. Fix all CRITICAL and HIGH confirmed findings before shipping.

## When to use

- `/ship` step 5 for large diffs (delegate from ship to here)
- Before merging a feature branch that touches shared contracts
- When you want a "second opinion" from fresh context

## Cost note

Each run: ~5 reviewers + up to N×3 verifiers. Budget ~50k-200k tokens for a medium diff. Use the single `code-reviewer` agent for small diffs instead.
