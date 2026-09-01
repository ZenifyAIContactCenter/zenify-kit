---
name: scout
description: Map what depends on something before you change it — the reverse question. Use before modifying existing code, an existing data shape, or anything shared: finds consumers, the tests that cover it, other systems written in the same operation, and why the code exists. Read-only and safe to run on its own. This is discovery; verifying that a name or shape is real is /ground's job, not this one.
argument-hint: "[symbol | collection | endpoint | file:line being changed]"
allowed-tools: Read Grep Glob Bash(git *) Bash(rg *) Agent
---

Scout: **$ARGUMENTS** (if no argument, scout whatever you are about to change).

## What this is, and what it is not

```
/ground   "what is X?"           verify a target you already know      → forward
/scout    "what depends on X?"   discover what you do not know yet     → outward
```

`/ground` protects you from using a name that does not exist. It does **nothing** to protect
you from breaking a caller you never knew existed — those are opposite directions, and no
amount of shape-checking substitutes for the second one.

Run this **before** editing existing code, not after. Running it after means you already made
the change and are now looking for permission.

## Dispatch the `scout` agent — do not search inline

Hand the work to the `scout` agent (Agent tool) and let it return a short `file:line` map.

This is not delegation for its own sake. Search generates a large volume of intermediate
output — candidate files, grep hits, history — and that volume landing in your own context has
a measured cost: routing search through a dedicated agent that returns short file:line lists
cut main-agent token use ~60% and **raised** accuracy. Searching inline means paying that cost
on purpose.

Tell the agent:
- what is about to change (symbol, collection, endpoint, or `file:line`)
- **which ref to scout** — the branch the change will land on. This matters: a hotfix branches
  from a release ref, and the consumers on that ref are not necessarily the consumers on the
  development branch. A map of the wrong branch looks entirely plausible.
- which of the four targets to emphasise, if the caller has a reason

### No report in hand means this step is not done

**Observed, not hypothetical, and not specific to this agent.** Across one session it happened
three times — twice with the `scout` agent, once with `claude-code-guide`. The agent goes idle,
the work is done, and nothing is delivered until you ask for it by name.

Because it spans agent types, **no instruction inside an agent definition can fix it.** A line
saying "your report is only delivered if you send it" was added to `agents/scout.md` and the next
run still went idle without reporting. The rule has to live here, on the caller's side, because
the caller is the only party that can tell the difference between a silent success and a silent
nothing.

That failure is dangerous in one specific way — from here it looks exactly like a scout that
found nothing. And "found nothing" is the reading you act on, so a lost report silently converts
into permission to proceed.

So treat the two as different states and never let one stand in for the other:

```
report in hand, empty sections  → scouted, nothing found      → proceed, noting it
no report in hand               → NOT SCOUTED                 → do not proceed
```

If the agent goes idle without a report, ask it for the report. If it cannot produce one, say
plainly that the scout did not complete — do not narrate it as a clean result.

**Same rule when a tool is unavailable rather than a report.** This skill and `/gate` both depend
on `Bash(rg *)`; if Bash is blocked — a permission gate down, a sandbox denial — neither can
search at all, and there is no independent grep tool to fall back on. That is a step that
**could not run**, which is a different state from a step that ran and found nothing. Report it as
blocked and say what is missing. The failure mode to refuse is the one where an unavailable tool
quietly becomes a clean bill of health.

### Check the receipt before you use the report

Four mechanical checks. Any one failing means send it back — one message, and it costs far less
than a decision made on a report you could not audit.

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

**Check 5 is the one that earns the whole list.** On the first run that satisfied checks 1-4, the
report declared `Searched: chat_message literal across all repos (grep -rl)` and then reported
`ott-gateway: no reference found (searched)`. Those cannot both be true — `chat_message` is a
substring of `chat_messages`, and that repo holds ~25 of them. One command settled it.

Note what this means about check 1: stating the search does not make the claim correct, it makes
it **auditable**. Without that line, "ott-gateway has no reference" is unfalsifiable short of
redoing the entire sweep. With it, the contradiction is visible in seconds. That is the value —
not fewer errors, but findable ones. Checks 1 and 6 exist to make 5 cheap.

**Check 3 has the worst failure mode and it is not theoretical.** A run of this reported a repo as
having "~1 spec file (per CLAUDE.md)". It had 95 — the doc was stale by 94 — and the report went on
to argue the repo was untested, which is an argument for skipping verification. A stale document
does not merely fail to help; it supplies a confident wrong premise, and it will supply the same
one to the next agent that reads it. When a doc turns out to be the source, **fix the doc too** —
otherwise you have corrected one report and left the cause in place.

**Do not expect the agent's own instructions to guarantee any of this.** Checks 1 and 2 were both
written into the agent definition, emphatically, and both were still violated on the next run.
Instructions to an agent are a request; this receipt check is the enforcement. That asymmetry is
the whole reason the check lives here, on the caller's side, rather than being assumed upstream.

## The four targets

| # | Target | Why |
|---|---|---|
| 1 | **Consumers** — who reads / writes / calls / publishes / subscribes | grepping the name finds the definition and little else; readers reach it through a symbol |
| 2 | **Tests covering the change** | strongest measured value of the four — see below |
| 3 | **Written together** (dual-write) | two systems written without one transaction fails **silently**; no error at the call site |
| 4 | **Why the code exists** (`git blame`, `git log -S`) | changing a line whose purpose you do not know is how a guard gets deleted |

Target 2 carries the numbers: an agent baseline broke **6.5 already-passing tests per patch**;
adding impact analysis to choose which tests a change could affect cut test regressions ~**70%**
and *raised* the resolution rate. Target 4 carries the incident: a global outage traced to a
refactor that had silently removed a CPU-time guard.

Target 4 matters most when the bug is in code **someone else** wrote — which is most bug
reports. You cannot recover the original author's intent from your own memory of not writing it.

## In this workspace

For target 1 on a **shared MongoDB collection, HTTP endpoint between services, BullMQ queue, or
Redis channel**, delegate to `/gate` instead of re-deriving it: `/gate` already carries the
eight-repo list and the three-pass search, and the two must not drift. `/scout` covers the rest
— tests, dual-write partners, line history — and the cases `/gate` does not model: a symbol
inside one repo, a component's props, a config key.

### Pin the repo list into the brief — never let the agent discover it

**Put the list in the dispatch and require a line per repo, including the empty ones.**

**Read the list out of `/gate` §1 at dispatch time. Do not reproduce it here.** That section is the
one source, and an earlier draft of this very paragraph enumerated all eight repos inline — which
made a second copy, in a file whose own instruction was not to make one. The list already lives in
`contract-sweep.js` and the `contract-sweep` skill as well; `/gate` notes that if those two ever
disagree, one has drifted. A fourth copy here would only make drift likelier and harder to spot.

So: open `/gate` §1, copy the repos into the brief for *this* dispatch, and let the file that owns
the list stay the only place it is maintained.

Why this is not optional: the same agent, the same brief, run three times, returned **three
different blast radii — 5, 8, then 5 repos.** Two of the three missed `ott-gateway` entirely,
which holds ~25 raw `database.getCollection("chat_messages")` calls in a single Java DAO.
Free-form discovery has run-to-run variance, and instructions do not remove it.

A project-local gate skill had already learned this and fixed it the same way — a pinned list,
carrying a note that two repos **used to be missing** and were added after someone swept the
shared store's name across the workspace. Those were the same two the scout runs dropped.

**Pin it once — and pin it in the project, not here.** A discovery that has to be complete should
not be rediscovered per run; that is the lesson. But the list belongs where it stays true: the
workspace's own `CLAUDE.md` or its gate skill. Pinning it into a global file makes it wrong in
every other project and stale in this one, which is exactly the failure this kit removed from
`contract-sweep`.

Requiring a line per repo is what turns a miss into a finding: a skipped repo becomes an empty row
you can see, instead of a sentence that simply never appears.

**Say in the brief what "depends on" means, or you will get the narrow answer.** It means *breaks
when this changes* — not *touches the database*. Two runs with an identical pinned list split on
exactly this: one reported all eight repos as dependents, counting the frontend's import of a
`MessageType` constant and the notification service's use of derived events; the other reported six,
because those two hold no database access. Neither misread the repos — they answered different
questions, and only the first is the question a shape change poses. `/gate` lists those two for
precisely that reason: they break **without touching Mongo**. Spell it out, since the narrower
reading is the more natural one for a search-shaped task.

### Blind spots worth naming in the brief

Three access styles reach the same collection — `@InjectModel(Class)`, a `models.mongo.*`
registry, and raw `.collection('name')` — so a search for the model symbol alone misses the third
entirely. Field names also hide inside aggregation stage strings, which no static analysis sees.
And **one repo is Java**: a search scoped to `.ts`/`.js` silently skips `ott-gateway`, which is
exactly how two of the first three runs lost it.

Name these in the brief as *kinds* of access to search for, not as counts. Counts of call sites
went stale and disagreed with each other every time they were measured here; the number never
changed what the search had to cover.

## A clean result is a claim, and it needs the same scepticism

If the agent reports no consumers, ask what could have hidden them before you believe it.
Reachability analysis has documented blind spots: reflection, dynamic dispatch, DI containers,
keys assembled from strings at runtime, and dependencies with no import edge at all (shared
mutable singletons, global config). The agent is instructed to write
**"cannot enumerate by grep: <mechanism>"** rather than reporting clean — if that line is
present, treat the result as partial and say so downstream.

A false "nothing depends on this" is worse than no answer, because it is acted on.

**And a hit is a claim too — check it before you overturn the report.** When you spot-check and
find something the report missed, read the hit before concluding the report was wrong. In that
first run the report said no test referenced `chat-message`; a re-search found
`conversation-messages.tool.spec.ts:20`, which turned out to contain the string only as a URL
path in a URL-encoding test — the report's *conclusion* (nothing constrains the document shape)
was right, its *wording* was not. Declaring "the scout gave a false clean" would have been the
opposite error, made in one step, from one unexamined grep line.

The asymmetry is only about which results let you proceed. A negative that opens the door needs
corroboration; a positive that would close it needs to be read before you act on it. Neither is
believed on sight.

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
