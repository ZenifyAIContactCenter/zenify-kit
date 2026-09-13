<!-- Moved verbatim from run/SKILL.md §§ intro, Step 3 (W4 slim-skills). Read when: you wonder why /run is a skill and not a bash line. -->

### From § intro
**This skill existed as a name before it existed as a file.** Eight places referenced `/run`
— including rule #3 itself and a `WORKFLOW.md` row claiming *"Agent-invocable, so skills can rely
on it"* — while `skills/run/` did not exist. The visible consequence: with nothing owning "which
port does this app use", agents improvised a port hunt, which silently discards the port `wt`
allocated and puts the app somewhere `znf:ui-verifier` is not looking.

### From § Step 3: The launch command comes from the project, not from memory
**Stop when the recipe is ambiguous, not merely when `CLAUDE.md` is absent** — that distinction cost
this skill its first run, where it was about to refuse a repo that had no `CLAUDE.md` and exactly
one dev script.

Filling the gap belongs to `/onboard-project`, which is already told to record the launch command
and port for exactly this reason.
