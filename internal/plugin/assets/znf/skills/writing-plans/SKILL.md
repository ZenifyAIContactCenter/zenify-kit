---
name: writing-plans
description: Use when you have a spec or requirements for a multi-step task, before touching code
---
<!-- Vendored from obra/superpowers (MIT). Adapted for the znf plugin. -->

# Writing Plans

## Overview

Write implementation plans for an engineer who has not seen this codebase or this spec. Assume they write idiomatic code in the project's language once they know the exact interface and the exact test, and that they will make a reasonable choice wherever the plan leaves one open. What they cannot know is what you decided: which files, which names and signatures, which values from the spec, which tests prove each task. Document those. Give them the whole plan as bite-sized tasks. DRY. YAGNI. TDD. Frequent commits.

**Announce at start:** "I'm using the writing-plans skill to create the implementation plan."

**Context:** the worktree is created with `wt new` (znf:discipline §8) at execution time.

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

**contract-frozen** = the other repo committed the endpoint + shape, not whole-repo-done. No `Waits for:` line = **independent**, runs in parallel. SDD schedules repos from this block ("Cross-worktree parallelism").

## File Structure

Before defining tasks, map out which files will be created or modified and what each one is responsible for — clear boundaries, one responsibility per file, files that change together live together. See `references/decomposition-rationale.md` for the full reasoning. This structure informs the task decomposition; each task should produce self-contained changes that make sense independently.

## Task Right-Sizing

A task is the smallest unit with its own test cycle that is worth a fresh reviewer's gate: fold setup/config/docs into the task that needs them; split only where a reviewer could reject one task and approve its neighbor. Each ends with an independently testable deliverable (`references/decomposition-rationale.md`). Splitting deserves depth: **Think hard before responding.** (steering sentence, unmeasured).

## Step Granularity

**Each step is one action with a checkable result:** write the failing test; run it, see it fail; implement the minimal code; run the tests, see them pass; commit.

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

[Spec's project-wide requirements (version floors, dependency limits, naming/copy rules, platform), one line each, exact values verbatim. Binds every task.]

## Review Focus

[Up to five inputs/failure modes the spec implies but no task's tests exercise, likeliest first: the condition and the expected behavior. Add each line's pinning test to the owning task.]

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
- `_Requirements: FR-N[, SC-M]_` — spec IDs (constitution P8); every task has one, every FR is in some task (`znf:analyze` checks). Own bullet line, backtick-wrapped.
- `_Skills: <list or none>_` — skills to invoke before first edit, per `znf:_shared/skill-routing`. Write `none` out; `znf:analyze` flags a missing or unknown tag.

- [ ] **Step 1: Write the failing test**

```python
def test_specific_behavior():
    result = function(input)
    assert result == expected
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pytest tests/path/test.py::test_name -v`
Expected: FAIL with "function not defined"

- [ ] **Step 3: Implement `function(input: InputType) -> ResultType` in `exact/path/to/file.py`**

One line on the approach when signature and test leave a choice; a code block only for an algorithm they do not determine.

- [ ] **Step 4: Run test to verify it passes**

Run: `pytest tests/path/test.py::test_name -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add tests/path/test.py src/path/file.py
git commit -m "feat: add specific feature"
```
````

## What a Step Contains

A step is done when the implementer can write exactly one reasonable thing from it: unambiguous, not complete.

- **Test step:** the test's name and assertions, as code, with the spec's exact values.
- **Code step:** exact signature, file, and the values the spec pins. The implementer writes the body; a body appears only for an algorithm the signature and tests do not determine, or exact copy the spec fixes.
- **Verification step:** the command and the output that means pass.
- **Reference to another task:** point at its Interfaces block; do not repeat its code.

A plan records decisions, not code. Forbidden placeholders: "TBD"/"TODO", "similar to Task N", a name no task defines, "add appropriate error handling".

## Necessity Note (fill only on violation)

Building more than the smallest thing that works: three lines (What is built / Why it is needed / Simpler alternative rejected because), see `references/decomposition-rationale.md`. Else omit.

## Self-Review

After writing the plan, check it against the spec with fresh eyes — a checklist you run yourself, not a subagent dispatch.

**1. Spec coverage:** Skim each section/requirement in the spec. Can you point to a task that implements it? List any gaps.

**2. Step scan:** a line that decides nothing is a gap; a body the signature and tests determine is a transcript. Fix both.

**3. Type consistency:** Do the types, method signatures, and property names you used in later tasks match what you defined in earlier tasks? A function called `clearLayers()` in Task 3 but `clearFullLayers()` in Task 7 is a bug.

**4. Review Focus:** each input class the spec implies has a task whose tests exercise it.

**5. Proportion:** a plan several times longer than its spec is a transcript; swap bodies for signatures and assertions.

Fix issues inline — no need to re-review. Add a task for any spec requirement that has none.

## Execution Handoff

After saving, give the absolute plan path and ask: **"Plan saved to `<abs path>`. Please approve the plan."**
Only after approval, use `znf:subagent-driven-development`. Under `/cook`, return to cook Step 5b (analyze + plan gate) instead.

## References

- `references/decomposition-rationale.md` — why files and tasks are sized this way; Review Focus and Proportion with an example.
