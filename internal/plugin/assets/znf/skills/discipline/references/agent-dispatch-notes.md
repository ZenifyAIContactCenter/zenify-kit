<!-- Moved verbatim from discipline/SKILL.md §§ 3, 5, 9 (W4 slim-skills). Read when: a dispatched agent went quiet, or the task-list tool is missing from the harness, or you are deciding how much to trust prose-only instructions. -->

### From § 3 — Verify before claiming done (idle-agent measurement)
- **A dispatched agent going idle is not a result.** Measured in one session: three times out of five dispatches, across two different agent types, the agent finished and its report never arrived — only an idle notification. Ask for the report by name; never read silence as "it ran and found nothing", because those two states are indistinguishable from here and only one of them is safe to act on. No instruction inside an agent definition fixes this: one was added and the next run behaved the same way.

### From § 3 — Verify before claiming done (the ledger file as the dispatch tracker)
- **The skill's ledger file from rule #5 is that tracker — one line per dispatched agent, ticked only once its report is in hand.** Held in your head instead, it is exactly what a context compaction drops, and losing it is silent. In the file, an agent that went quiet stays visible as an unticked line; off it, that agent leaves no trace at all, and "no trace" reads identically to "nothing to report".

### From § 5
- **The harness task list (TodoWrite / TaskCreate) stays off — do not re-enable it.** Measured over four sessions (2026-09-20): every list call ran as its own API turn, never batched with other tools, and those turns took 7–18% of all tool-turn spend (cache reads plus thinking) while the list content itself was under 3% of tool bytes. The ledger file gives the same observability — a quiet agent is an unticked line — and survives compaction, which the list does not. Recent Claude models ship with the tools off by default (`CLAUDE_CODE_ENABLE_TODO_TOOLS` unset); leave it that way.

### From § 9
- Prose is a nudge, not a guarantee — instruction files get truncated or ignored at length. Where a
  tool allows it, back this with a deterministic gate on the *output* (a triggered turn must carry a
  real, fetched citation), because no gate can force the search itself. Keep the rule short.
