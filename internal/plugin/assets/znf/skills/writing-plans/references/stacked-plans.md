# Stacked plans

Read when the plan may ship as a stack of dependent PRs instead of one PR.

## When to propose a stack

- The expected diff exceeds ~600 lines of code, or
- the tasks group into independent layers that each build and test on their own.

Only propose it. The user decides at `znf:cook` Step 5b; without their approval the plan has no `## Stack` block and ships as one PR.

## Format

Add a `## Stack` block after `## Review Focus` and before the `---` that precedes the first task. One line per layer, bottom layer first:

```markdown
## Stack

- Layer 1: `store-schema` — `feat(store): add the session table`
- Layer 2: `store-api` — `feat(api): expose session reads`
```

Each task carries one `**Layer:** <N>` line directly under its `**Model:**` line, outside the `**Files:**` block:

```markdown
### Task 3: Session read endpoint

**Model:** sonnet
**Layer:** 2

**Files:**
- Create: ...
```

## Rules

- One task belongs to exactly one layer.
- A task in layer N depends only on tasks in layers <= N. A task that needs a higher layer's output is misplaced; re-decompose.
- The block must sit before the first task. `znf:analyze`, `znf:standards` and the task brief assign every line after the last task heading to that task, so a block placed later is read as part of the last task.
- No `## Stack` block means no task has a `**Layer:**` line, and the plan follows the one-PR flow unchanged.
