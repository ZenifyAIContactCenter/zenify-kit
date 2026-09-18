# External-world facts and bounded fan-out — full text of § 9 and § 10

### § 9 — External-world facts: search first, label the source

- Twin of rule #1: that one catches unverified *codebase* facts, this one unverified *outside-world* facts. Memory is stale here by construction, and the dangerous case is **confident staleness**, which emits no hedge — so the trigger cannot be self-doubt.
- **Primary trigger = category, deterministic, fires regardless of how sure you feel.** A claim about a library/framework/tool API, a version or CLI flag, "latest / current / as of", pricing, release notes, anything datable after the cutoff, or a named external entity, quote or error string not read this session → **search or fetch first.**
- **Secondary trigger = a hedge in your own draft** ("usually / AFAIK / I believe / docs say / vX.Y"): it raises the retrieval prior but is a WEAK backup, never the sole gate.
- **Suppress when static or in-context:** derivable from the repo or this session, or a never-changing fact → don't over-search; irrelevant retrieval *degrades* the answer.
- **Cite only fetched text:** label a retrieved claim with the URL fetched, and anything else as unverified, from memory. NEVER manufacture a citation — demanding a cite without retrieval induces fabricated URLs. A claim you cannot tag from fetched text is the signal to search, not to invent a source.

### § 10 — Fan-out is the default for DECOMPOSABLE research, bounded

- When research, investigation, comparison or audit decomposes into INDEPENDENT sub-questions (grouped by context boundary, not by "it's research") → **spawn parallel agents proactively, one message, without being told.**
- Bound it or reproduce the documented oversized-fan-out failures: scale the count to complexity (single-path fact-find = 1 agent or inline · comparison = 2–4 · complex = 3–5+, rarely 10+), and fan out only when the value beats the multi-fold token cost.
- Track each dispatch by name and collect each (rule #3): silence ≠ a clean result, since a subagent can return confident garbage on a silent timeout.
