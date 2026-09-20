# Decomposition rationale

Why file structure and task sizing are decided the way they are, expanded from the SKILL.md summary.

## File Structure

Before defining tasks, map out which files will be created or modified and what each one is responsible for. This is where decomposition decisions get locked in.

- Design units with clear boundaries and well-defined interfaces. Each file should have one clear responsibility.
- You reason best about code you can hold in context at once, and your edits are more reliable when files are focused. Prefer smaller, focused files over large ones that do too much.
- Files that change together should live together. Split by responsibility, not by technical layer.
- In existing codebases, follow established patterns. If the codebase uses large files, don't unilaterally restructure - but if a file you're modifying has grown unwieldy, including a split in the plan is reasonable.

This structure informs the task decomposition. Each task should produce self-contained changes that make sense independently.

## Task Right-Sizing

A task is the smallest unit that carries its own test cycle and is worth a fresh reviewer's gate. When drawing task boundaries: fold setup, configuration, scaffolding, and documentation steps into the task whose deliverable needs them; split only where a reviewer could meaningfully reject one task while approving its neighbor. Each task ends with an independently testable deliverable.

## Necessity Note forcing function

This is the plan-time forcing function against over-engineering (adapted from spec-kit's Complexity Tracking): a deviation from the smallest thing that works must name the simpler thing it rejected and why.

## Necessity Note (fill only on violation)

Constitution P6 (necessity ladder) applies at plan time too. If a task builds MORE than the
smallest thing that works — a new abstraction, a new dependency, an extra layer — justify it in
three labeled lines, and only then:

- What is built: the extra abstraction / dependency / layer
- Why it is needed: the concrete reason the smallest thing does not suffice
- Simpler alternative rejected because: why the one-line / stdlib / existing-path option fails

No violation → omit the section.
