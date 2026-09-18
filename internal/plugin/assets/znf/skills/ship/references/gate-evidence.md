<!-- Moved verbatim from ship/SKILL.md § What this gate is actually for, § Working tree — fingerprint the tree, not HEAD, § The fix loop — it wraps every check, not just the review, § Output (W4 slim-skills). Read when: a clean review tempts you to skip step 4, or someone asks why the board exists. -->

**Contents:** what this gate is for · fingerprinting the tree · the fix loop · recording the outcome · pushing the branch · why the board is `cat`ed · the board template.
### From § What this gate is actually for
Be honest about which parts carry weight, because the evidence is not flattering to the part that
costs most.

**The load-bearing parts are the cheap deterministic ones** — lint, build, the contract gate,
behavioural verification, and the fingerprint below. They either produce output or they don't.

**The independent review is the expensive part with the weakest evidence.** A peer-reviewed
replication on real defect data (Empirical SE 2020, arXiv:2005.09217) found models *without* review
predictors fit post-release defects as well or better; prior defect count, module size and authorship
dominate every review metric. Microsoft's own study of review at scale (Bacchelli & Bird, ICSE 2013)
found the observed payoff leans toward code understanding and knowledge transfer rather than
bug-catching. So run the review, but do not treat it as what makes the change safe — and do not let
a clean review substitute for step 4.

**Which means this gate has to prove it is not ceremony.** DORA 2019 found organisations requiring
external approval were 2.6× more likely to be low performers, and observed change-approval boards
that approved over 90% of changes — some rejecting nothing at all in a year. A gate with a ~0%
rejection rate is not a gate. Step 7 records whether this run blocked, so that question stays
answerable.

### From § Working tree — fingerprint the tree, not HEAD
**Do not stamp checks with `git rev-parse HEAD`.** This gate commits at the *end*, so during every
check the working tree is dirty by definition and HEAD is not what you tested. Verified: modifying a
tracked file, and adding an untracked one, both leave HEAD unchanged. `git stash create` is not a fix
either — it ignores untracked files.

### From § Start the agents before the inline work — the two costs
**A failing check voids the concurrent work.** Any fix changes `fp`, so every stamp taken in that
round is VOID and the loop re-runs them. Concurrency pays off on the pass path — which is the
common one — and is wasted on the fail path. Worth it, not free.

3 of 5 dispatches in one measured session finished without their report arriving (`CLAUDE.md §3`).
With four out at once, expecting all four back unprompted is optimistic.

### From § The fix loop — it wraps every check, not just the review
This used to live inside step 5, which meant a behaviour fix was never re-reviewed and a lint fix had
no defined re-verification at all — even though both are code changes.

**Two rounds, not five.** SDD's five-round cap is for a development loop over one task, with a ledger
and later tasks still to run. This is the last gate: the next action is a push. If two attempts cannot
close a CRITICAL, the problem is in the design, and that is the user's call.

**There is no "park with ruling" here.** SDD can park a finding because a final review reads the
ledger afterwards. Nothing downstream of `/ship` reads anything. The only honest exits are: closed, or
handed to the user.

**Do not expect the loop to catch fix-induced regressions reliably.** A study of 97,347 Firefox pull
requests found 12.2% introduced new bugs *despite* passing lint, tests, regression tests and code
review, and multi-file fixes regressed more often. No study isolates what a re-review adds for this
case. The loop exists because without it the board prints ✅ earned on a diff that no longer exists —
that is a smaller and provable claim.

### From § Step 7: Record the outcome, then commit
That is the measured signature of an approval board that approves everything — and the honest
conclusion would be that these checks are costing time without filtering anything, not that the
work has been flawless.

### From § Output — pushing the feature branch
House rule #7 authorises this without asking: pushing a feature branch deploys nothing.

### From § Output
This is the difference between an instruction and a mechanism, and the instruction alone does not hold:
three attempts in this toolkit to change behaviour with prose all failed, and the fixes that worked were
structural. A board that must be `cat`ed is **output from a command** — the caller cannot compose a
shorter version of it from memory, a skipped `cat` leaves a visible hole where a tool call should be, and
the file is still on disk afterwards for the user to diff against what was reported.

Why it matters that this not collapse: `/cook`, `/fix` and `/hotfix` all end here, so if each summarises
the board as *"`/ship`: ✅ all green"*, everything that carries weight in the entire pipeline becomes one
word, and the per-check fingerprints — the only part the user can check without trusting me — disappear.
Every line goes in the file, including the ❌ ones, the skipped ones, and `Gate log`.

That is also the honest answer to *"why is this a skill instead of steps in each pipeline?"* — one copy
of the logic, three copies of the **output**. Duplicating the checks into three files would let them
drift silently; duplicating the board cannot drift, because it is generated here each run.

### The board template (moved verbatim from § Output)
Fill every line; write it to the board file and `cat` it. Never retype or summarise it.

```
Verified at fingerprint = <fp10>

✅/❌ Lint                (<fp10>)  <command run>
✅/❌ Build / typecheck    (<fp10>)  <command run>
✅/❌ Contract gate        (<fp10>)  /gate: <N repos impacted, or clean>
✅/❌ Behaviour verified   (<fp10>)  <N tests passed — or what /run showed>
      look: <verdict + overflow numbers · "nothing renders" ONLY if rg=0 · or "❌ BLOCKED: <missing thing>" → Shippable NO>
            before concluding: `zenify ui-verify check --repo <path> --base <base>` — non-zero →
            append "❌ BLOCKED" here and Shippable NO; a waiver appends "waived: <reason>"
      data checks: <which of the project-specific ones ran; which the diff could not trigger>
✅/❌ Independent review   (<fp10>)  round <R>: <N CRITICAL/HIGH → addressed> · diff <N> LOC
      not blocking: <MEDIUM/LOW findings, plus any out-of-scope observations>
      deferred from the ledger: <N minor · which the reviewer called must-fix-before-merge,
                                 or "no ledger" for a /fix or /hotfix run>
Deploy order: ...
Gate log: <SHIPPED|BLOCKED> · blocked <N> of last <M> runs

Shippable: YES only if every fingerprint above == the current one
```

Only call it shippable when every applicable line is ✅ at the current fingerprint, each backed by
real output. A check whose agent went quiet never earns a ✅ — silence and a clean result are
indistinguishable from here.

### Ordering the gate (moved from § Start the agents)
Log in first — that is blocking, because neither verifier can authenticate itself. Then dispatch the
contract gate's sweeps and `znf:ui-verifier` in ONE message so they run at once, and do lint, build
and the data checks inline while they work. `Skill(znf:review)` cannot start earlier: it needs the
facts those checks produce.
