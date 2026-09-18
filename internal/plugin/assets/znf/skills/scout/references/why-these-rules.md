# Scout — why each rule exists

Evidence behind the rules in `SKILL.md`. Read when a rule looks like overhead.

- [Why dispatch instead of searching inline](#why-dispatch-instead-of-searching-inline)
- [Why a missing report is not a clean result](#why-a-missing-report-is-not-a-clean-result)
- [Why the receipt checks exist](#why-the-receipt-checks-exist)
- [Why the four targets are those four](#why-the-four-targets-are-those-four)
- [Why the repo list is pinned, and pinned in the project](#why-the-repo-list-is-pinned-and-pinned-in-the-project)
- [Why a clean result gets the same scepticism](#why-a-clean-result-gets-the-same-scepticism)

## Why dispatch instead of searching inline

Search generates a large volume of intermediate output — candidate files, grep hits, history —
and that volume landing in the main context has a measured cost. Routing search through a
dedicated subagent that returns short `file:line` lists cut main-agent token use ~60% and
**raised** accuracy. Searching inline means paying that cost on purpose.

## Why a missing report is not a clean result

Observed, not hypothetical, and not specific to one agent type. Across one session it happened
three times — twice with the `scout` agent, once with `claude-code-guide`. The agent goes idle,
the work is done, and nothing is delivered until you ask for it by name.

Because it spans agent types, **no instruction inside an agent definition can fix it.** A line
saying "your report is only delivered if you send it" was added to the scout agent definition and
the next run still went idle without reporting. The rule has to live on the caller's side,
because the caller is the only party that can tell a silent success from a silent nothing.

The danger is specific: from the caller's seat, a lost report looks exactly like a scout that
found nothing. "Found nothing" is the reading you act on, so a lost report silently converts into
permission to proceed.

The same asymmetry applies when a tool is unavailable rather than a report. This skill and
`/gate` both depend on `Bash(rg *)`; if Bash is blocked — a permission gate down, a sandbox
denial — neither can search at all, and there is no independent grep tool to fall back on. A step
that **could not run** is a different state from a step that ran and found nothing. The failure
mode to refuse is the one where an unavailable tool quietly becomes a clean bill of health.

## Why the receipt checks exist

**Check 5 earns the whole list.** On the first run that satisfied checks 1-4, the report declared
`Searched: chat_message literal across all repos (grep -rl)` and then reported
`ott-gateway: no reference found (searched)`. Those cannot both be true — `chat_message` is a
substring of `chat_messages`, and that repo holds ~25 of them. One command settled it.

Note what this means about check 1: stating the search does not make the claim correct, it makes
it **auditable**. Without that line, "ott-gateway has no reference" is unfalsifiable short of
redoing the entire sweep. With it, the contradiction is visible in seconds. That is the value —
not fewer errors, but findable ones. Checks 1 and 6 exist to make 5 cheap.

**Check 3 has the worst failure mode and it is not theoretical.** A run of this reported a repo as
having "~1 spec file (per CLAUDE.md)". It had 95 — the doc was stale by 94 — and the report went
on to argue the repo was untested, which is an argument for skipping verification. A stale
document does not merely fail to help; it supplies a confident wrong premise, and it will supply
the same one to the next agent that reads it. When a doc turns out to be the source, **fix the
doc too** — otherwise you have corrected one report and left the cause in place.

**Do not expect the agent's own instructions to guarantee any of this.** Checks 1 and 2 were both
written into the agent definition, emphatically, and both were still violated on the next run.
Instructions to an agent are a request; the receipt check is the enforcement.

## Why the four targets are those four

Target 2 carries the numbers: an agent baseline broke **6.5 already-passing tests per patch**;
adding impact analysis to choose which tests a change could affect cut test regressions ~**70%**
and *raised* the resolution rate.

Target 4 carries the incident: a global outage traced to a refactor that had silently removed a
CPU-time guard. It matters most when the bug is in code **someone else** wrote — which is most
bug reports. You cannot recover the original author's intent from your own memory of not writing
it.

## Why the repo list is pinned, and pinned in the project

The same agent, the same brief, run three times, returned **three different blast radii — 5, 8,
then 5 repos.** Two of the three missed `ott-gateway` entirely, which holds ~25 raw
`database.getCollection("chat_messages")` calls in a single Java DAO. Free-form discovery has
run-to-run variance, and instructions do not remove it.

A project-local gate skill had already learned this and fixed it the same way — a pinned list,
carrying a note that two repos **used to be missing** and were added after someone swept the
shared store's name across the workspace. Those were the same two the scout runs dropped.

The list belongs where it stays true: the workspace's own `CLAUDE.md` or its gate skill. Pinning
it into a global file makes it wrong in every other project and stale in this one. An earlier
draft of the skill enumerated all eight repos inline — a second copy, in a file whose own
instruction was not to make one. The list already lives in `contract-sweep.js` and the
`contract-sweep` skill as well.

Requiring a line per repo is what turns a miss into a finding: a skipped repo becomes an empty row
you can see, instead of a sentence that simply never appears.

**Why "depends on" has to be spelled out.** Two runs with an identical pinned list split on
exactly this: one reported all eight repos as dependents, counting the frontend's import of a
`MessageType` constant and the notification service's use of derived events; the other reported
six, because those two hold no database access. Neither misread the repos — they answered
different questions, and only the first is the question a shape change poses. `/gate` lists those
two for precisely that reason: they break **without touching Mongo**. The narrower reading is the
more natural one for a search-shaped task.

**Why kinds of access, not counts.** Counts of call sites went stale and disagreed with each other
every time they were measured here; the number never changed what the search had to cover.

## Why a clean result gets the same scepticism

Reachability analysis has documented blind spots: reflection, dynamic dispatch, DI containers,
keys assembled from strings at runtime, and dependencies with no import edge at all (shared
mutable singletons, global config). A false "nothing depends on this" is worse than no answer,
because it is acted on.

**And a hit is a claim too.** In the first run the report said no test referenced `chat-message`;
a re-search found `conversation-messages.tool.spec.ts:20`, which turned out to contain the string
only as a URL path in a URL-encoding test — the report's *conclusion* (nothing constrains the
document shape) was right, its *wording* was not. Declaring "the scout gave a false clean" would
have been the opposite error, made in one step, from one unexamined grep line.

The asymmetry is only about which results let you proceed. A negative that opens the door needs
corroboration; a positive that would close it needs to be read before you act on it.
