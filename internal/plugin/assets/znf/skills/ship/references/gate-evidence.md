<!-- Moved verbatim from ship/SKILL.md § What this gate is actually for, § Working tree — fingerprint the tree, not HEAD, § The fix loop — it wraps every check, not just the review, § Output (W4 slim-skills). Read when: a clean review tempts you to skip step 4, or someone asks why the board exists. -->

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

### From § The fix loop — it wraps every check, not just the review
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
