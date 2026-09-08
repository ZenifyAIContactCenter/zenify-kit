---
name: standards
description: Use after implementing a plan — checks test-traceability: every FR/SC has a real test on disk, not just a testable-shaped SC. Mechanical command + judgment on whether the test truly asserts the requirement. advisory only, never blocks.
allowed-tools: Read Bash(zenify standards *)
---

# znf:standards — every requirement has a real test

**Announce:** "Using znf:standards to check test-traceability."

M5b checks that an SC **is shaped as** testable; this skill checks that the requirement **has a real
test** in the code — runs **after implementation**. Cross-checks FR/SC ↔ the test file declared in
the plan, verified on disk. **Advisory:** reports findings, does NOT block progress. Two layers —
mechanical (command) then judgment (skill).

## When to use

- After SDD has finished implementing the plan, before/at ship (cook calls it at Step 6b).
- Or invoke by hand: `/standards <spec.md> <plan.md>` on an already-implemented pair.

## Step 1 — mechanical scan (deterministic)

```
zenify standards --spec <spec-path> --plan <plan-path> --root <repo-root>
```

The command reuses the FR→task coverage from `znf:analyze`, plus checks the test file on disk:
- `untested-fr` (HIGH) — an FR has a covering task, but that task declares no test.
- `missing-test-file` (HIGH) — the test path declared in the plan doesn't exist on disk.
- `empty-test-file` (MEDIUM) — the test file exists but has no test func (language-aware).
- `unchecked-lang` (INFO) — unrecognized extension, only existence is checked, not content.

Fails open: the command always exits 0, never blocks.

## Step 2 — judgment (2 passes, MEDIUM, done by the skill — the command can't do this)

- **Pass A — does the test ACTUALLY assert the requirement.** Read (`Read`) a few test files that
  were NOT flagged: an empty `func TestX` or one that's just `assert True`/`expect(true)` still passes
  Step 1 but checks nothing. Report which tests exist but have an empty/trivial assertion relative to
  the FR they're attached to.
- **Pass B — does each SC Given/When/Then have a corresponding assertion.** Cross-check the SC in the
  spec against the assertion in the test: a When/Then branch with no corresponding test is a gap, even
  when the overall FR "has a test."

## Step 3 — advisory report

Open with: "Advisory — does not block progress." List mechanical + judgment findings, one line each
(kind · FR/path · why). Never blocks; the decision-maker accepts or fixes.
