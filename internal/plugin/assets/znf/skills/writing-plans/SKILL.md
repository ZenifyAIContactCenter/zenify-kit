---
name: writing-plans
description: Use when you have a spec or requirements for a multi-step task, before touching code
---
<!-- Vendored from obra/superpowers (MIT). Adapted for the znf plugin. -->

# Writing Plans

## Overview

Write comprehensive implementation plans assuming the engineer has zero context for our codebase and questionable taste. Document everything they need to know: which files to touch for each task, code, testing, docs they might need to check, how to test it. Give them the whole plan as bite-sized tasks. DRY. YAGNI. TDD. Frequent commits.

Assume a skilled developer who knows nothing about our toolset, problem domain, or good test design.

**Announce at start:** "I'm using the writing-plans skill to create the implementation plan."

**Context:** If working in an isolated worktree, it should have been created with `wt new` (znf:discipline §8) at execution time.

**Save plans to:** `docs/superpowers/plans/YYYY-MM-DD-<feature-name>.md`
- (User preferences for plan location override this default)

**Author plans against the shared references.** Format/language/diagram rules follow
`znf:_shared/artifact-style` (a plan is as much a human-read artifact as the spec); plan
discipline — stable IDs, testable steps, traceability — follows `znf:_shared/constitution`.

## Scope Check

If the spec covers multiple independent subsystems, it should have been broken into sub-project specs during brainstorming. If not, suggest separate plans — one per subsystem — each producing working, testable software on its own.

**When the plans span several repos, declare the dependency edges.** Each sub-plan opens with a
small block naming its repo and what it must wait for:

```
Repo: <repo name>
Waits for: <other repo> : contract-frozen      (omit this line if the repo is independent)
```

The edge is **contract-frozen** — the other repo has committed the endpoint + shape, not
whole-repo-done. A sub-plan with no `Waits for:` line is **independent** and runs in parallel from
the start. Subagent-Driven Development reads this block to schedule repos — independent ones
concurrently, dependent ones gated on contract-freeze. See its "Cross-worktree parallelism (polyrepo)" section.

## File Structure

Before defining tasks, map out which files will be created or modified and what each one is responsible for — clear boundaries, one responsibility per file, files that change together live together. See `references/decomposition-rationale.md` for the full reasoning. This structure informs the task decomposition; each task should produce self-contained changes that make sense independently.

## Task Right-Sizing

A task is the smallest unit that carries its own test cycle and is worth a fresh reviewer's gate — fold setup/config/scaffolding/docs into the task whose deliverable needs them, split only where a reviewer could meaningfully reject one task while approving its neighbor. Each task ends with an independently testable deliverable. See `references/decomposition-rationale.md` for why.

## Bite-Sized Task Granularity

**Each step is one action (2-5 minutes):**
- "Write the failing test" - step
- "Run it to make sure it fails" - step
- "Implement the minimal code to make the test pass" - step
- "Run the tests and make sure they pass" - step
- "Commit" - step

## Plan Document Header

**Every plan MUST start with this header:**

```markdown
# [Feature Name] Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use znf:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** [One sentence describing what this builds]

**Architecture:** [2-3 sentences about approach]

**Tech Stack:** [Key technologies/libraries]

**Spec:** [path to the spec/design doc this plan implements — the plan
argues from the spec, so the spec travels with it; executors read both]

## Global Constraints

[The spec's project-wide requirements — version floors, dependency limits,
naming and copy rules, platform requirements — one line each, with exact
values copied verbatim from the spec. Every task's requirements implicitly
include this section.]

---
```

## Task Structure

````markdown
### Task N: [Component Name]

**Files:**
- Create: `exact/path/to/file.py`
- Modify: `exact/path/to/existing.py:123-145`
- Test: `tests/exact/path/to/test.py`

**Interfaces:**
- Consumes: [what this task uses from earlier tasks — exact signatures]
- Produces: [what later tasks rely on — exact function names, parameter
  and return types. A task's implementer sees only their own task; this
  block is how they learn the names and types neighboring tasks use.]
- `_Requirements: FR-N[, SC-M]_` — the spec requirement IDs this task implements (constitution
  P8 traceability). Every task carries one; every FR in the spec must appear in at least one
  task's line. This is what lets a coverage check assert no requirement is orphaned and no task
  is unmotivated — `znf:analyze` (cited at `cook` Step 5b) is that check. Keep the tag on its
  own bullet line, in this backtick-wrapped form, so the check detects it.

- [ ] **Step 1: Write the failing test**

```python
def test_specific_behavior():
    result = function(input)
    assert result == expected
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pytest tests/path/test.py::test_name -v`
Expected: FAIL with "function not defined"

- [ ] **Step 3: Write minimal implementation**

```python
def function(input):
    return expected
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pytest tests/path/test.py::test_name -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add tests/path/test.py src/path/file.py
git commit -m "feat: add specific feature"
```
````

## No Placeholders

Every step must contain the actual content an engineer needs. These are **plan failures** — never write them:
- "TBD", "TODO", "implement later", "fill in details"
- "Add appropriate error handling" / "add validation" / "handle edge cases"
- "Write tests for the above" (without actual test code)
- "Similar to Task N" (repeat the code — the engineer may be reading tasks out of order)
- Steps that describe what to do without showing how (code blocks required for code steps)
- References to types, functions, or methods not defined in any task

## Necessity Note (fill only on violation)

Constitution P6 (necessity ladder) applies at plan time too. If a task builds MORE than the
smallest thing that works — a new abstraction, a new dependency, an extra layer — justify it in
three labeled lines, and only then:

- What is built: the extra abstraction / dependency / layer
- Why it is needed: the concrete reason the smallest thing does not suffice
- Simpler alternative rejected because: why the one-line / stdlib / existing-path option fails

No violation → omit the section. See `references/decomposition-rationale.md` for why.

## Self-Review

After writing the plan, check it against the spec with fresh eyes — a checklist you run yourself, not a subagent dispatch.

**1. Spec coverage:** Skim each section/requirement in the spec. Can you point to a task that implements it? List any gaps.

**2. Placeholder scan:** Search your plan for red flags — any of the patterns from the "No Placeholders" section above. Fix them.

**3. Type consistency:** Do the types, method signatures, and property names you used in later tasks match what you defined in earlier tasks? A function called `clearLayers()` in Task 3 but `clearFullLayers()` in Task 7 is a bug.

Fix issues inline — no need to re-review. Add a task for any spec requirement that has none.

## Execution Handoff

After saving the plan, hand off to execution:

**"Plan complete and saved to `docs/superpowers/plans/<filename>.md`. Executing via Subagent-Driven Development** - I dispatch a fresh subagent per task, review between tasks, fast iteration.

**REQUIRED SUB-SKILL:** Use znf:subagent-driven-development
- Fresh subagent per task + two-stage review

## References

- `references/decomposition-rationale.md` — why file structure and task right-sizing are decided the way they are.
