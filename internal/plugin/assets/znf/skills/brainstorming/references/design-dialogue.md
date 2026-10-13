<!-- Moved verbatim from brainstorming/SKILL.md § The Process (token-diet). Read when: running the dialogue itself — what to ask, how to propose approaches, how to present a design, how to work in an existing codebase. -->

### The process, in detail

The subsections below serve the bounded and architectural paths (a
spike stops at "present the probe, get a nod"). Sections from
**Exploring approaches** onward are architectural-path depth — for
bounded work, context plus a few questions plus a short in-chat design
is the whole process.

**Shared understanding first (architectural; bounded gets the same in one message):**

1. **Discover intent.** From the request and context, identify the intended outcome, who it is for, and what success looks like. If that is missing, ask one focused question about purpose before proposing features or an approach. Knowing the app genre does not tell you why your partner wants it. Gathering requirements is not asking them to authorize the task again.
2. **Write back your understanding.** Summarize outcome, constraints and success criteria in a short note. Separate what they said from your assumptions, invite correction, and fold the answer in before treating it as the design brief. The corrected note becomes the Brief's Problem field.
3. **Carry intent into the design.** Preserve the agreed understanding in the path's artifact (the spec for architectural, the in-chat design for bounded). Check each proposed feature and technical choice against it.

When the request already states purpose and constraints, reflect them back instead of re-asking. Keep the note short; its accuracy and the chance to correct it are what matter.

**Understanding the idea:**

- Check out the current project state first (files, docs, recent commits)
- If the workspace has `.claude/GLOSSARY.md`, read it when a domain term in the request is unclear.
- Before asking detailed questions, assess scope: if the request describes multiple independent subsystems (e.g., "build a platform with chat, file storage, billing, and analytics"), flag this immediately. Don't spend questions refining details of a project that needs to be decomposed first.
- If the project is too large for a single spec, help the user decompose into sub-projects: what are the independent pieces, how do they relate, what order should they be built? Then brainstorm the first sub-project through the normal design flow. Each sub-project gets its own spec → plan → implementation cycle.
- For appropriately-scoped projects, batch the questions that are ready into one round: at most 4, recommended answer first, each with why it matters
- If the repo or DB can answer a question, look it up instead of asking
- A topic that needs deeper exploration goes to a later round
- Prefer multiple choice questions when possible, but open-ended is fine too
- Focus on understanding: purpose, constraints, success criteria

**Exploring approaches:**

- Propose 2-3 different approaches with trade-offs, each driven by a different constraint; do this in chat, never by dispatching a subagent
- Present options conversationally with your recommendation and reasoning
- Lead with your recommended option and explain why
- YAGNI ruthlessly - remove unnecessary features from every approach and design

**Presenting the design:**

- Once you believe you understand what you're building, present the design
- Scale each section to its complexity: a few sentences if straightforward, up to 200-300 words if nuanced
- Ask after each section whether it looks right so far
- Cover: architecture, components, data flow, error handling, testing
- Be ready to go back and clarify if something doesn't make sense

**Design deep modules:**

- Apply the deletion test: imagine deleting the module. If the complexity vanishes, it was a pass-through; if it reappears across its callers, it earned its place.
- The interface is the test surface: if you cannot test a module through its interface alone, the boundary is in the wrong place.
- A seam with one adapter is hypothetical; add no abstraction until a second real implementation exists.
- An interface includes its invariants and errors, not only its signatures: what must hold before and after, and how it fails.

**Working in existing codebases:**

- Explore the current structure before proposing changes. Follow existing patterns.
- Where existing code has problems that affect the work (e.g., a file that's grown too large, unclear boundaries, tangled responsibilities), include targeted improvements as part of the design - the way a good developer improves code they're working in.
- Don't propose unrelated refactoring. Stay focused on what serves the current goal.
