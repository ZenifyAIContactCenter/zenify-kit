<!-- Moved verbatim from fix/SKILL.md §§ How much diagnosis, worktree, Step 1, Step 3, Step 4, Step 8 (token-diet). Read when: tempted to skip a step of /fix, or asked why /fix insists on scout + ship. -->

## Contents
- Why the worktree step sits before Step 5, and why there is no workspace handoff
- Why the Wide path uses one agent per hypothesis
- Why a fix needs `/scout` more than a feature does
- Why the escalation door to `/cook` exists
- Why `/ship` runs unconditionally
- The five clean checks that were wrong

### From § How much diagnosis
It never skips a step, and in particular it never skips Step 2 — you state the confirmed root
cause, with the evidence that confirmed it, before writing any fix, on both paths. "It looked
simple" is how you end up fixing the wrong thing, which Step 0 exists to prevent.

### From § Isolation
**No workspace handoff here, unlike `/cook` — and the reason is the artifact.** `/cook` can move
Step 6 into a workspace of its own because what crosses over is a **plan file** that
`writing-plans` already requires to be self-contained. `/fix`'s equivalent is the *diagnosis*, and
that lives in this conversation: Step 0-3's evidence, the hypotheses killed, the one that survived.
Hand that to a fresh session and it starts by re-deriving what you already proved.

**A bug found while another task is in progress is not a new task.** This is the most common way
`/fix` gets entered — mid-feature, from inside a worktree that already exists. Then the fix belongs
in that worktree: `cd` there and skip this step. `wt new` refuses a second task in the repo anyway
and prints where to go. Only a bug genuinely unrelated to the work in flight earns `--another`, and
a live production bug is `/hotfix`, which is exempt by design.

### From § Step 1: why parallel investigators
This is the largest single latency in `/fix` and the reason it is worth agents rather than inline
searching: each hypothesis needs a broad sweep whose intermediate output (file lists, log greps,
match dumps) is large and worthless once the verdict is known. Investigating three hypotheses
inline lands all three sweeps in this context; three agents land three verdicts. Dispatching them
in a single message is what makes them run concurrently; separate messages run them one after
another and buy nothing. You already stated the evidence that settles each hypothesis, which makes
this a search against a fixed target rather than open-ended reasoning. Keep the reasoning at the top
tier where it belongs: forming the hypotheses and confirming the survivor, both inline.

Confirming the surviving hypothesis is Step 2, and it stays **inline** — that step reads real error
output, and its output is the evidence the fix is built on, not a map pointing at it.

### From § Step 3: why scouting a fix matters more than scouting a feature
**Invoke it as a tool, not as an intention.** `Skill(znf:scout)` and the `Agent(znf:scout)` it
dispatches each leave a line carrying their name; grounding or scouting merely *performed*
dissolves into a scatter of `Bash`/`Read` calls indistinguishable from any other work. That
difference decides whether "did the scout run?" is answerable by looking or only by trusting the
summary. **A missing line is a skipped step.**

A fix is riskier than a feature in exactly this respect: a feature adds code nothing calls yet,
while a fix changes a path that is live enough for someone to have watched it fail. The numbers
are from bug-fixing benchmarks specifically — an agent baseline broke **6.5 already-passing
tests per patch**, and impact-analysis-guided test selection cut that ~**70%** while *raising*
the resolution rate.

Target 4 (why these lines exist) carries most of the weight, because **the bug is usually in code
someone else wrote**. You have no memory of the intent encoded in those lines, and the obvious fix
may delete exactly the case the original author was handling. A global outage was traced to a
refactor that silently removed a CPU-time guard: nobody knew why it was there.

### From § Step 4: the escalation door
At more than ~3 files, a changed cross-service contract, or a real design decision, it is a
feature-shaped change and the label "it's a bug" stops mattering — it needs a spec, a plan, and
per-task review, none of which exist in this skill. Continuing here means writing a multi-file
behavioural change inline with no independent review of the implementation.

### From § Step 6: this step vs /ship step 4
Here the question is narrow: *did the bug go away*. `/ship` step 4 is the authoritative pass at the
final fingerprint, after any review fix, and it is the only place a UI verifier agent is dispatched.
If the bug was visual, looking at it in the browser here also leaves the browser **logged in**,
which `/ship` step 4 then inherits: neither verifier agent can authenticate itself. What you do
**not** do here is the objective layout measurement or the screenshot audit — that is the agent's
job at step 4, and duplicating it costs a second login handoff for no new information.

### From § Step 6: the five clean checks that were wrong
In one session, five separate checks reported clean and the clean was wrong: `grep -R` missed
a line `rg` and `grep -c` both found; `+show-config` said the setting applied while text was
clipped off-screen; a wrapper reported "not a git worktree" where two existed. No amount of
review catches this class — reviewers read the same tool output you did.

### From § Step 8: why /ship is unconditional
This was previously conditional on the gate, which left a single-repo fix with **no independent
review at all**: gate clean → commit → push. Agents report success when they have not succeeded:
**75.8%** of trajectories carrying a self-reported status claim were false successes, and an LLM
judge asked to detect that reaches only **AUROC 0.65** — barely better than a coin flip, so a
self-check cannot close it. What did close it was independent verification outside the agent's own
control: **48% → 3%** across comparable domains. `/ship`'s reviewer is that independent party. The
cost is one agent call on a small diff. Weigh it against 6.5 broken tests per patch.

### From § Step 7: why the board file is pasted
Do not retype it and do not collapse it to *"`/ship`: ✅"*: that hides every check that carries
weight in this pipeline and discards the per-check fingerprints, which are the only part the user
can verify without trusting this report. A `cat` also makes a skipped step visible as a missing
tool call.
