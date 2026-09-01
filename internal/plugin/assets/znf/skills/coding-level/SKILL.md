---
name: coding-level
description: Set the output calibration level for this session. Use when you want more or less explanation, comments, and conceptual density in responses. Persists for the rest of the session.
argument-hint: "[0-5]"
disable-model-invocation: false
---

# Coding Level Calibration

Set the output style for this session. Valid levels: **0** through **5**.

If no argument given, show the current level and describe each option.

## Levels

| Level | Audience | Comments | Explanation | Abstractions |
|---|---|---|---|---|
| **0** | Expert / pair-programmer | Minimal, only non-obvious | None — code speaks | Advanced patterns OK |
| **1** | Senior dev | Key decisions only | Brief rationale | Standard idioms |
| **2** | Mid-level dev (default) | Meaningful names + key steps | "Why" not "what" | Common patterns |
| **3** | Junior / learning | More comments | Step-by-step rationale | Explicit, no magic |
| **4** | Beginner | Heavy comments | Full explanation | No shortcuts |
| **5** | Teaching mode | Every line explained | Pedagogical, with examples | Concepts introduced |

## Setting the level

When invoked as `/coding-level 0`:
- Acknowledge: "Output level set to 0 (Expert). Minimal comments, no hand-holding."
- Apply for the **rest of this session** — affects all code output, explanations, and PR descriptions

When invoked with no argument:
- State the current level (default: 2)
- Show the table above
- Ask which level to set

## How it applies

- **Code comments**: scale from near-zero (0) to line-by-line (5)
- **Explanation prose**: scale from one-liner rationale (0) to full walkthrough (5)  
- **Abstractions**: level 0-1 uses advanced patterns freely; level 3-5 makes patterns explicit
- **PR/commit messages**: level 0 = terse conventional commit; level 5 = educational prose

This does NOT affect correctness requirements, verification, or security checks — those are always full regardless of level.
