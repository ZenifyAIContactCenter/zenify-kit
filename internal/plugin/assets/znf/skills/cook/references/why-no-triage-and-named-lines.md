<!-- Moved verbatim from cook/SKILL.md § There is no complexity triage, § Every step must leave a
named line (W4 slim-skills). Read when: you want to call something "too simple for /cook" or skip
a named line. -->

### From § There is no complexity triage

This skill used to open with a Simple/Complex assessment that let a "Simple" feature skip
brainstorming. That is deleted, deliberately, for two reasons:

- `znf:brainstorming` forbids it in a section titled **"Anti-Pattern: 'This Is Too
  Simple To Need A Design'"**, plus a HARD-GATE that applies "to EVERY project regardless
  of perceived simplicity". The old Step 1 was the exact rationalization that skill names.
- What looks simple often isn't, and the judgement is made before the information that
  would settle it exists.

### From § Every step must leave a named line

The reason is auditability, not tidiness. A step invoked as a tool renders one line carrying its
name; a step merely *performed* dissolves into a scatter of `Bash(zenify db-read …)` and `Read(…)` calls
that look like every other piece of work. Then "did the grounding actually happen?" is answerable
only by trusting the summary — and the whole design of this pipeline is that its steps can be
checked by looking, the way `Agent(znf:scout)` can.

It costs something real — invoking a skill reloads its text into context, and `/ground` runs three
times here. Pay it. An unverifiable step is worth less than the tokens it saved.

### Where verification sits (moved from § What `/cook` does, token diet 2026-09-18)

**Verification sits around `brainstorming` and `writing-plans`, never inside them.** The checks go
before and after; neither skill is reordered or overridden.

`/ground` asks "what is X?" at each of the three points a name enters (request, spec, plan);
`/scout` asks "what depends on X?" once, after the spec decides what changes. Neither substitutes
for the other.

Mechanical work — nothing unknown, one repo, no shared resource — belongs on the spine in
`CLAUDE.md §0`, not in `/cook`.
