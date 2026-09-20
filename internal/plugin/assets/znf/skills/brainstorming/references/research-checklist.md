# Research check — run once, before the design is locked

Answer the five items after the clarifying questions and before proposing approaches. **One item
true → invoke `Skill(znf:research)`** with the scoped question (`<question> repo: <feature-repo>`)
and wait for its return. **None true → skip**, and do not write a prior-art section for show.

1. The design depends on an external library, framework, tool or API whose documentation was not
   read in this session.
2. The design rests on "how others do X" — a pattern, community practice, or a competitor's
   product.
3. A fact that goes stale is load-bearing: "latest" or "current", a version number, pricing, a
   release note, anything datable after the model's knowledge cutoff.
4. Two or more genuinely different approaches are on the table and choosing wrong costs more than
   one research run (about 5 USD).
5. Reverse check: the unknown is a file, contract, field, symbol, collection or config key inside
   the repository or its database → that is `/znf:scout` or `/znf:ground`, **not** research; treat
   this item as false.

## Using the result

`znf:research` returns at most 40 lines and a file under `docs/reference/<repo>/`. Link that file
from the Brief's Approach field, quote only rows the verifier marked `VERIFIED`, and carry every
`not found` into the spec as a stated assumption. A claim flagged `QUOTE-MISMATCH` or `DEAD-URL`
does not enter the spec.

Why a checklist and not a fixed step: across 88 specs in one team store, 9 needed web research,
all of them tooling or process designs; product features almost never do. spec-kit gates its
research phase the same way — only on fields still marked as needing clarification.
