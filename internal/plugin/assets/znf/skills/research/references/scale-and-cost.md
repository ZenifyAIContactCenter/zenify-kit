# Scale and cost

## Worker count by question shape

| Shape | Workers | Tool-call cap per worker | Example |
|---|---|---|---|
| Fact-find: one answer, one source family | 1 | 3–10 | "What does flag X do in tool Y's current release?" |
| Comparison: 2–4 options or systems | 2–4 (one per option) | 10–15 | "How do A, B and C handle retries?" |
| Complex: many sub-systems or a survey | 5+ (rare; justify in the report) | 10–15 each | "How do published deep-research tools structure their pipelines?" |

Source of the table: Anthropic's multi-agent research write-up
(https://www.anthropic.com/engineering/multi-agent-research-system) — simple fact-finding uses
one agent with 3–10 calls, direct comparisons 2–4 subagents with 10–15 calls each, complex
research more than ten subagents. Start with the smaller count; a second round is cheaper than an
over-wide first one.

Split **by context boundary** (one source family, one product, one system per worker), never by
keyword. Two workers reading the same documentation set duplicate work and produce conflicting
quotes.

## Verifier

Always one, on the cheapest tier (`model: haiku`). It fetches, it does not judge: dead URLs and
mismatched quotes are mechanical to detect, so a miss is cheap and visible. Reading and
synthesis never run on that tier.

## Cost estimate (state it in the scope block)

Measured in this kit's cost baseline (2026-09-18, `zenify cost`): a sonnet research worker
averages about 1.6 USD per run; a haiku verifier well under 1 USD. Typical comparison research
(3 workers + verifier) lands near 5–6 USD. Re-measure with `zenify cost --by-skill` when the
numbers matter; they drift with page sizes and model pricing.

## When not to run

The unknown is a file, function, field, collection, endpoint or config key inside the
repository or its database. That is `/znf:ground` (what is X?) or `/znf:scout` (what depends on
X?), and web research would return confident noise.
