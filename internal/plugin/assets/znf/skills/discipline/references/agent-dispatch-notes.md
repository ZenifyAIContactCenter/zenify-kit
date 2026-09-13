<!-- Moved verbatim from discipline/SKILL.md §§ 3, 5, 9 (W4 slim-skills). Read when: a dispatched agent went quiet, or the task-list tool is missing from the harness, or you are deciding how much to trust prose-only instructions. -->

### From § 3
- **A dispatched agent going idle is not a result.** Measured in one session: three times out of five dispatches, across two different agent types, the agent finished and its report never arrived — only an idle notification. Ask for the report by name; never read silence as "it ran and found nothing", because those two states are indistinguishable from here and only one of them is safe to act on. No instruction inside an agent definition fixes this: one was added and the next run behaved the same way.

### From § 3
- **The plan/TodoWrite list from rule #5 is that tracker — one item per dispatched agent, ticked only once its report is in hand.** Held in your head instead, it is exactly what a context compaction drops, and losing it is silent. On the list, an agent that went quiet stays visible as an unticked line; off it, that agent leaves no trace at all, and "no trace" reads identically to "nothing to report".

### From § 5
- **On newer models you must turn this list on — the tool is off by default.** Recent Claude models track multi-step work internally, so the harness omits the task-tracking tools (TodoWrite and the Task tools) by default to save context; a fresh session then has no list tool at all, presented as "no such tool" rather than "disabled" — which is exactly the silent-absence trap. This rule still binds, because the list is also **human observability**: a reader watching dispatched agents return, which the context saving does not replace. Re-enable with `CLAUDE_CODE_ENABLE_TODO_TOOLS=1` set **before the session starts** (a restart, not a live toggle), and reveal the on-screen panel with the interactive task-panel toggle (Ctrl+T in current builds). Keep it on for supervised work; let unattended/batch runs drop it. Env-var names and defaults shift by version — re-verify against current harness docs.

### From § 9
- Prose is a nudge, not a guarantee — instruction files get truncated or ignored at length. Where a
  tool allows it, back this with a deterministic gate on the *output* (a triggered turn must carry a
  real, fetched citation), because no gate can force the search itself. Keep the rule short.
