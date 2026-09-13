<!-- Moved verbatim from ship/SKILL.md § 5. Independent review, § Start the agents before the inline work (W4 slim-skills). Read when: you are tempted to write a verdict into `## Verified`, drop `## Deferred`, or dispatch the reviewer early. -->

### From § 5. Independent review — the ship-pack fields
**`## Deferred` is here because this gate is the last reader.** SDD parks minor findings in the
ledger for its final whole-branch review to triage, and nothing downstream of `/ship` ever opens that
file again — SDD names the risk itself: *"a roll-up nobody reads is a silent discard."* Handing the
list to a reviewer that already has the diff in front of it is the cheapest place to notice one that
turned out not to be minor.

These are **not** findings of this review and they do **not** enter the fix loop. The reviewer's only
job with them is one line each: must-fix-before-merge, or fine to leave. Whatever it says goes on the
board at step 7, so the list reaches you either way — that is the actual fix, since the failure mode
was never bad triage, it was the list being read by nobody.

`## Intent` is there because reviewers without the intent approve code that is syntactically fine
and solves the wrong problem — the throughline across the Microsoft, Chromium and Firefox studies.

**`## Verified` carries facts, not a verdict, and the distinction is the whole point.** Without it
the reviewer flags "there is no test for this" when a test exists, which costs a full loop round.
But writing "✅ Behaviour verified" instead of the numbers invites agreement and stops the reviewer
asking the better question — *are these tests adequate?* Anchoring in code review is an observed
phenomenon whose magnitude on outcomes nobody has measured, so this is an unquantified risk taken on
voluntarily; facts let the reviewer judge, a verdict asks it to concur. Write `12/12 pass —
pm test src/modules/auth`, name the spec files, and **name the changed behaviour no test covers** —
that last line is the one most likely to earn its place.

This is also why the review runs *after* behavioural verification and not before. Reordering would
cut rework — but there is no evidence that check order changes what gets caught, only what it costs,
and it would leave this block empty.

`## Ground` is what lets it check field names against reality; the agent has no Bash of its own, by
design, so the data must be handed to it.

### From § Start the agents before the inline work — why the reviewer is last
**Why the reviewer is last and stays last.** Its ship-pack's `## Verified` block is *what steps 2-4
actually produced* — the real command output, which test files ran by name, and which changed
behaviour no test touches. Those facts do not exist until the inline checks have run. Dispatching it
earlier means building that block from expectation instead of output, which is the exact substitution
the block exists to prevent: a reviewer that cannot see what went untested approves code that is
syntactically fine and unverified. Concurrency is not worth buying with that.

The win is still where the time actually goes: the gate's eight per-repo sweeps are the slowest thing
in this gate, and they now run underneath lint and build instead of after them.
