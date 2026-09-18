<!-- Moved verbatim from subagent-driven-development/SKILL.md § 2. Handle the report, § 1. Dispatch the implementer, § 3. Review the task, § 4. The fix loop, § Final Review, § Finish (W4 slim-skills). Read when: a fix loop passes round 3, you want to pre-judge a reviewer, or you consider skipping the rulings list. -->

### From § 2. Handle the report

it prints the unique file path it wrote; BASE is the commit you recorded
before dispatching the implementer —

which silently drops all but the last commit of a multi-commit task),
then dispatch the task reviewer with the printed path.

The implementer completed the work but flagged doubts.

If the concerns are about correctness or scope, address them before
review. If they're observations (e.g., "this file is getting large"), note
them and proceed to review.

The implementer needs information that wasn't provided.

If the implementer said it's stuck, something needs to change.

If the implementer asks questions — before starting or mid-task — answer
clearly and completely, provide additional context if needed, and don't
rush it into implementation.

### From § 1. Dispatch the implementer

a real session's dispatch hit 42k chars of which 99%
was pasted history.

In real sessions, every reviewer a worker spawned duplicated
the task review the controller dispatched anyway — a full extra
review seat per task.

### From § 3. Review the task

The output never enters your own context, and the reviewer sees
the commit list, stat summary, and full diff with context in one Read
call.

The broad review happens once, at the
final whole-branch review.

The reviewer's template already carries the process rules (YAGNI,
test hygiene, review method) — the constraints block is for what THIS
project's spec demands.

If you believe a finding would be a
false positive, let the reviewer raise it and adjudicate it in the review
loop.

If the prompt you are writing contains "do not flag," "don't treat X
as a defect," "at most Minor," or "the plan chose" — stop: you are
pre-judging, usually to spare yourself a review loop.

### From § 4. The fix loop

A roll-up nobody reads is a silent discard.

you hold the plan and
the cross-task context the reviewer lacks:

Its context is intact: it knows the task, the code, and its own
choices.

the report file is the persistent memory either way.

A loop that survives three resumes usually means the implementer cannot see its
own problem — fresh eyes and a capability bump in one move.

your context stays
clean for coordination, and controller fixes skip review.

a one-line fix does not need the
whole suite.

Adjudicate only at the cap. Adjudicating earlier to end a loop is
pre-judging with a different name.

The final review sees both sides.

Parking a structural failure
silently lets every dependent task build on it. Stop only when the defect
leaves every path forward a guess.

### From § Final Review

Skipped under `/cook` since 2026-09-18: every diff used to meet three reviewers (task, SDD final,
ship) and the middle one recorded no review-driven fix in the ledgers measured, while each pass
re-read the whole branch diff. `/ship`'s engine review keeps the whole-branch pass and inherits the
ledger's deferred and parked lines via the ship-pack.

Per-finding fixers each rebuild context and re-run suites; a real
session's final-review fix wave cost more than all its tasks combined.

### From § Finish

That list is the only place the decisions you
took on your human partner's behalf reach them — they read it and rework
whatever you got wrong. A ruling that dies with the workspace was a decision
made in secret.

### From § The Task Loop

Everything you paste into a dispatch prompt — and everything a subagent
prints back — stays resident in your context for the rest of the session
and is re-read on every later turn.

A bounded stretch keeps nearly
all of a long wait's efficiency while guaranteeing a stuck or lost
child is noticed within minutes, not at the end of the session.

This is where the wall-clock
is won: the dependent repo overlaps the rest of X.
