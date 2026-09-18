---
name: scout
description: Map what depends on something before you change it — the reverse question. Use before modifying existing code, an existing data shape, or anything shared: finds consumers, the tests that cover it, other systems written in the same operation, and why the code exists. Read-only and safe to run on its own. This is discovery; verifying that a name or shape is real is /ground's job, not this one.
argument-hint: "[symbol | collection | endpoint | file:line being changed]"
allowed-tools: Read Grep Glob Bash(git *) Bash(rg *) Agent
---

Scout: **$ARGUMENTS** (if no argument, scout whatever you are about to change).

Tool names for each action: `znf:_shared/harness-tools` (harness mapping table).

## What this is, and what it is not

```
/ground   "what is X?"           verify a target you already know      → forward
/scout    "what depends on X?"   discover what you do not know yet     → outward
```

`/ground` protects you from using a name that does not exist. It does **nothing** to protect
you from breaking a caller you never knew existed.

Run this **before** editing existing code, not after. Running it after means you already made
the change and are now looking for permission.

## Dispatch the `scout` subagent — do not search inline

Hand the work to the `scout` subagent and let it return a short `file:line` map. Searching
inline costs ~60% more main-context tokens and lowers accuracy (see references).

Tell the agent:
- what is about to change (symbol, collection, endpoint, or `file:line`)
- **which ref to scout** — the branch the change will land on. A hotfix branches from a release
  ref, and the consumers there are not the consumers on the development branch. A map of the
  wrong branch looks entirely plausible.
- which of the four targets to emphasise, if the caller has a reason

### No report in hand means this step is not done

Treat these as different states and never let one stand in for the other:

```
report in hand, empty sections  → scouted, nothing found      → proceed, noting it
no report in hand               → NOT SCOUTED                 → do not proceed
```

If the agent goes idle without a report, ask it for the report. If it cannot produce one, say
plainly that the scout did not complete — do not narrate it as a clean result. Same when a tool
is unavailable rather than a report: report it as blocked and say what is missing.

### Check the receipt before you use the report

Six mechanical checks. Any one failing means send it back.

```
1  Does every "none found" state the patterns that were searched?
     no  → unfalsifiable. You cannot tell a real absence from a missed search.
2  Is it within the length cap, with an `Omitted:` line if it had to cut?
     no  → the cap exists to protect this context; over it, delegating gained nothing.
3  Is any countable claim sourced to documentation rather than the filesystem?
     yes → reject that line. Counts in docs go stale silently.
4  Does it say which ref it scouted, separately from which repos had no consumers?
     no  → you cannot tell "searched and empty" from "never searched".
5  Would the search it declared actually produce the result it reported?
     no  → the report contradicts itself. Re-run that search yourself.
6  Is there a line for every repo you pinned, including the empty ones?
     no  → a repo with no row was not covered, whatever the prose says.
```

## The four targets

| # | Target | Why |
|---|---|---|
| 1 | **Consumers** — who reads / writes / calls / publishes / subscribes | grepping the name finds the definition and little else; readers reach it through a symbol |
| 2 | **Tests covering the change** | impact analysis cut test regressions ~70% |
| 3 | **Written together** (dual-write) | two systems written without one transaction fails **silently**; no error at the call site |
| 4 | **Why the code exists** (`git blame`, `git log -S`) | changing a line whose purpose you do not know is how a guard gets deleted |

## In this workspace

For target 1 on a **shared MongoDB collection, HTTP endpoint between services, BullMQ queue, or
Redis channel**, delegate to `/gate` instead of re-deriving it: `/gate` already carries the
eight-repo list and the three-pass search, and the two must not drift. `/scout` covers the rest
— tests, dual-write partners, line history — and the cases `/gate` does not model: a symbol
inside one repo, a component's props, a config key.

### Pin the repo list into the brief — never let the agent discover it

**Put the list in the dispatch and require a line per repo, including the empty ones.**

**Read the list out of `/gate` §1 at dispatch time. Do not reproduce it here.** That section is
the one source; the list also lives in `contract-sweep.js` and the `contract-sweep` skill, and a
fourth copy would only make drift likelier. Open `/gate` §1, copy the repos into the brief for
*this* dispatch, and let the file that owns the list stay the only place it is maintained.

**Pin it in the project, not here** — the workspace's own `CLAUDE.md` or its gate skill.

**Say in the brief what "depends on" means, or you will get the narrow answer.** It means *breaks
when this changes* — not *touches the database*. A repo that only consumes derived events or an
exported constant still breaks.

### Blind spots worth naming in the brief

Three access styles reach the same collection — `@InjectModel(Class)`, a `models.mongo.*`
registry, and raw `.collection('name')` — so a search for the model symbol alone misses the third
entirely. Field names also hide inside aggregation stage strings, which no static analysis sees.
And **one repo is Java**: a search scoped to `.ts`/`.js` silently skips `ott-gateway`.

Name these in the brief as *kinds* of access to search for, not as counts.

## A clean result is a claim, and it needs the same scepticism

If the agent reports no consumers, ask what could have hidden them before you believe it:
reflection, dynamic dispatch, DI containers, keys assembled from strings at runtime, shared
mutable singletons. The agent is instructed to write
**"cannot enumerate by grep: <mechanism>"** rather than reporting clean — if that line is
present, treat the result as partial and say so downstream.

**And a hit is a claim too — read it before you overturn the report.** A spot-check that appears
to contradict the report may be a string match with no dependency behind it.

## Output

Pass the agent's report through, trimmed to what the caller needs:

```
Scouted: <target>  @ <ref>
Blast radius: <N files, M repos>
Highest risk: <the one thing most likely to break — file:line>
Uncovered by tests: <changed behaviour with no covering test>
Why the code exists: <sha> <subject>
Confidence: high | partial — <what could not be determined>
```

Then say what you will do differently because of it. A scout report that changes nothing about
the plan was either unnecessary or unread.

## References

- `references/why-these-rules.md` — why dispatch, why a missing report is not a clean result, and the incidents behind the receipt checks, the four targets and the pinned repo list.
