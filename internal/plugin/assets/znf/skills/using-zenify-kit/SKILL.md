---
name: using-zenify-kit
description: Use when starting any conversation - establishes how to find and use the znf kit skills, requiring skill invocation before ANY response including clarifying questions
---
<!-- Adapted from obra/superpowers (MIT), reframed for the znf kit. -->

<SUBAGENT-STOP>
If you were dispatched as a subagent to execute a specific task, ignore this skill.
</SUBAGENT-STOP>

<EXTREMELY-IMPORTANT>
If you think there is even a 1% chance a `znf:*` skill might apply to what you are doing, you ABSOLUTELY MUST invoke the skill.

IF A SKILL APPLIES TO YOUR TASK, YOU DO NOT HAVE A CHOICE. YOU MUST USE IT.

This is not negotiable. You cannot rationalize your way out of this.
</EXTREMELY-IMPORTANT>

## The Rule

**Invoke relevant or requested skills BEFORE any response or action** — including clarifying questions, exploring the codebase, or checking files. If it turns out wrong for the situation, you don't have to use it.

**Before entering plan mode:** if you haven't already brainstormed, invoke `znf:brainstorming` first.

Then announce "Using [skill] to [purpose]" and follow the skill exactly. If it has a checklist, create a todo per item.

## Route the work — pick the skill by what is unknown

The znf kit is a workflow, not a grab-bag. Route by what you do not know yet; `znf:discipline` carries the full doctrine (blast radius, worktree, gate), this is the short reminder:

| Unknown | Skill |
|---|---|
| what to build, or how — no agreed design | `znf:cook` |
| it's broken and you don't know why | `znf:fix` |
| "what is X?" — verify a name/shape you will use | `znf:ground` |
| "what depends on X?" — before changing existing code | `znf:scout` |
| a shared resource changed (DB collection, endpoint, queue, channel) | `znf:gate` |
| ready to ship — lint, verify, review, contract-check | `znf:ship` |
| nothing unknown, one repo, the what and why are both stated | no skill — just the discipline floor |

When in doubt about the floor (worktree-before-edit, verify-before-done, no-fabrication), invoke `znf:discipline`.

## Skill Priority

When multiple skills apply, process skills come first — they set the approach, then implementation skills carry it out. `znf:brainstorming` and `znf:fix` are the kit's most common process skills, but the rule holds for any of them.

- "Let's build X" → `znf:brainstorming` first, then implementation skills.
- "Fix this bug" → `znf:fix` first, then domain skills.

## Red Flags

These thoughts mean STOP—you're rationalizing:

| Thought | Reality |
|---------|---------|
| "This is just a simple question" | Questions are tasks. Check for skills. |
| "I need more context first" | Skill check comes BEFORE clarifying questions. |
| "Let me explore the codebase first" | Skills tell you HOW to explore. Check first. |
| "I can check git/files quickly" | Files lack conversation context. Check for skills. |
| "Let me gather information first" | Skills tell you HOW to gather information. |
| "This doesn't need a formal skill" | If a skill exists, use it. |
| "I remember this skill" | Skills evolve. Read current version. |
| "This doesn't count as a task" | Action = task. Check for skills. |
| "The skill is overkill" | Simple things become complex. Use it. |
| "I'll just do this one thing first" | Check BEFORE doing anything. |
| "This feels productive" | Undisciplined action wastes time. Skills prevent this. |
| "I know what that means" | Knowing the concept ≠ using the skill. Invoke it. |

## User Instructions

User instructions (CLAUDE.md, direct requests) take precedence over skills, which in turn override default behavior. Only skip skill workflows or instructions when your human partner has explicitly told you to.
