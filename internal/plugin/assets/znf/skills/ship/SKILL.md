---
name: ship
description: Pre-ship gate. Use when work is complete and about to be committed — runs lint/build on the changed areas, the cross-service contract gate, behavioural verification, and an independent review, with one fix-and-re-verify loop over all of them, then commits and pushes the feature branch. Does not open the PR. Invoked unconditionally by /cook, /fix and /hotfix.
allowed-tools: Bash(git *) Bash(pm *) Bash(db_read *) Bash(rg *) Bash(printf *) Bash(cat *) Bash(tail *) Bash(wc *) Read Grep Agent
---

## What this gate is actually for

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

## Working tree — fingerprint the tree, not HEAD

**Do not stamp checks with `git rev-parse HEAD`.** This gate commits at the *end*, so during every
check the working tree is dirty by definition and HEAD is not what you tested. Verified: modifying a
tracked file, and adding an untracked one, both leave HEAD unchanged. `git stash create` is not a fix
either — it ignores untracked files.

```bash
git status --short && git diff --name-only        # what changed
fp() { { git rev-parse HEAD; git diff HEAD; git status --porcelain -uall; } \
         | git hash-object --stdin | cut -c1-10; }
fp                                                # the stamp; changes on any tree change
```

Stamp every check with `fp` at the moment it ran. Step 7 recomputes `fp` and requires every stamp to
equal it. That is the whole honesty mechanism: it makes "this ✅ was earned on the code you are about
to push" mechanically checkable instead of something the user has to take on trust.

## Steps

Steps 3 and 4 are **started** before step 2's inline checks, though their results are read in this
order — see "Start the agents before the inline work" after step 6. Step 5 is the exception: it cannot
start early, and that section says why.

1. **Scope**: from the changed files, list which projects/areas changed.

2. **Static checks**: for each changed area, run its lint and build/typecheck and report the
   real output. **Do not assume `npm`** — use `pm`, which resolves the manager from the
   `packageManager` field then the lockfile, and refuses rather than guessing:
   `pm run lint`, `pm run build`. Never claim a pass without the output.

3. **Contract gate**: if the change touched anything shared across services — a DB collection, an
   HTTP endpoint between services, a queue, a pub/sub channel — run the project's contract gate,
   where it defines one, and report which repos it found. Do not decide by judgement that a change
   "looks local"; the gate answers that.

4. **Behavioural verification**: confirm the change actually works. Tests if they exist; otherwise
   **`Skill(znf:run)`** and observe the real code path — "it builds" is not "it works". Invoke it as a
   tool: it reads the port this worktree was allocated rather than hunting for a free one, and the
   URL it reports is what `ui-verifier` needs. A `Skill(znf:run)` line is checkable; "I ran the app" is
   the shape that dissolves into unnamed `Bash` calls.

   **This is the authoritative UI pass — the last one, not the only one.** `/ship` is invoked
   unconditionally by `/cook`, `/fix` and `/hotfix`, so a rendering change always reaches it without the
   step being copied into three pipelines. It runs *here* because it must happen at the **final
   fingerprint**: a review fix in step 5 can break layout, and a check taken before that fix certifies a
   tree that no longer exists.

   `/cook` step 6 also looks, once per rendering task, before writing that task's ledger line. The two do
   not overlap: the per-task check tells you **which task** broke a layout, and cannot see a layout broken
   three tasks later; this one catches what a *review* fix breaks, and cannot attribute it to a task.
   Neither substitutes for the other, and `/fix` and `/hotfix` have no per-task equivalent, so for them
   this is genuinely the only look.

   **Trigger it mechanically, not by judgement:**

   ```bash
   git diff --name-only HEAD | rg -c '\.(tsx|jsx|vue|svelte|css|scss|less)$|components?/|pages?/|views?/'
   ```

   Non-zero → dispatch **`ui-verifier`** (`~/.claude/agents/ui-verifier.md`) and **do not drive the
   browser yourself**. Zero → write "nothing renders in this diff" on the board and move on. The agent
   exists for exactly this and says why in its own description: it *"keeps heavy browser output out of
   the main context"*.

   **Log in yourself first, then hand the live session over.** Neither verifier can get past a login:
   they have the eight ordinary browser tools and **not** `browser_run_code_unsafe`, and their attempt to
   read credentials is classifier-blocked. So the main session logs in — `browser_snapshot` for the refs,
   then `browser_type` into the fields and `browser_click` the button, with values read from the
   workspace `settings.local.json` — and only then dispatches the verifier onto the already-authenticated
   browser. Two things this avoids, both of which have actually happened: a verifier stalling for a human
   because it could not authenticate, and a verifier being handed an *old* session whose screens are
   empty by design, so the check verified nothing. If a `browser_type` carrying a password is refused
   once with *"Stage 2 classifier error — usually transient, retrying often succeeds"*, that means what it
   says: retry, do not start building a way around it. Tell the verifier **not** to clear
   `localStorage` or cookies — one logged itself out mid-run. Snapshots, console dumps and screenshots are the largest
   volume any step here produces, and it returns a verdict plus evidence instead. Tell it the dev URL,
   how to log in, which screen, and what changed; require **both** a screenshot **and** a measurement of
   the changed element against its own container box (`getBoundingClientRect`: `child.right` vs
   `container.right − paddingRight`) — page-level scroll is not enough, since a child can spill an inner
   panel without producing a scrollbar. Ask it to compare against an unchanged sibling, so a spill can
   be attributed to this change rather than to something pre-existing.

   Two constraints from that agent's own caveat: the Playwright browser is a **single shared instance**,
   so never run two browser-driving agents at once and **do not touch Playwright yourself while it
   runs** — and if it reports the browser already in use, another window holds it. Also: its report may
   not arrive on its own (see `CLAUDE.md §3`); ask for it, and never read silence as a clean look.

   **Keep the output you keep small, but keep it real.** `pm run build` and lint on a large repo
   produce far more than you need — pipe them (`2>&1 | tail -20`, or grep the error lines) rather than
   delegating them. An agent would hand back its *summary* of the output, and step 2 exists precisely to
   look at the output; moving the evidence one hop away to save context trades the wrong thing.

   **A suite that ran nothing is not a pass.** `0 tests`, `passWithNoTests`, "No tests found" — all
   render as green and mean nothing. Report the test count, and if it is zero say so and fall back to
   `Skill(znf:run)`.

   **A clean check is not yet evidence.** A positive result carries its own content — "FAIL",
   "error", "3 matches" means something standing alone. A *negative* one — "no match", "0 results",
   "OK", "nothing found", "no diff" — that lets you **proceed** does not, until a second independent
   mechanism agrees:

   | The check said | Confirm it with |
   |---|---|
   | a repo-wide sweep found 0 hits | `rg`, never `grep -R`; plus one count against a file known to contain a hit |
   | the setting/config was applied | measure the effect (pixels, `getBoundingClientRect`, real output) — not by re-reading the config |
   | a wrapper tool: "not found" / "none" / "not a repo" | run the underlying tool directly |
   | a connection failed | `nc -z <host> <port>` first — separate network from credential before theorising. **Sandbox disabled, and say so:** inside it `nc`/`curl` call every port closed. Local port → `lsof -nP -iTCP:<port> -sTCP:LISTEN` |

   This targets one asymmetry that keeps producing false greens, and it has a documented instance
   outside this project: a CI provider's instrumentation returned `null` instead of a file, the null
   "appeared to be a valid response", the pipeline **registered the failures as successes**, no alert
   fired, and it was caught 3.5 hours later by someone reading a dashboard.

   **Three data checks — cheap, and almost never run.** Each catches a class that
   development-sized data hides completely, so passing tests say nothing about them.

   **Trigger them mechanically, not by remembering which project you are in:**

   ```bash
   command -v db_read >/dev/null || echo "no db_read on PATH — these three do not apply here"
   git diff HEAD | rg -c '\.find\(|\.aggregate\(|\.skip\(|OFFSET|findOne\(|updateMany\('
   ```

   Both true → run them. Either false → say which and why, and move on. The condition is written as a
   command on purpose: it self-disables in a project without `db_read`, instead of relying on you to
   read a "skip unless the project matches" note and act on it. That note was the earlier version of
   this paragraph, and a note is exactly the form that four separate attempts today failed to make
   stick.

   Report which you ran and which the diff could not trigger.

   - **The diff adds or changes a DB query → read its plan** (`COLLSCAN` / `Seq Scan` on a large
     collection or table is the finding). Delegate the full size-aware rubric to
     **`Skill(znf:explain-plan)`** — it greps the query call-sites, runs
     `db_read eval '…explain("executionStats")'` / `EXPLAIN ANALYZE` per site, and reports which
     scan a full table. An index existing does not mean it is used (non-selective field, wrong
     compound-index column order, `$in`/`$or`). Advisory — it does not block.
   - **The diff touches a query on a tenant-scoped collection → assert the negative.** Query tenant
     B's context against a row known to belong to tenant A and assert **zero rows**. One test case,
     and it exercises the whole mandatory-filter path. Nothing substitutes for it: a query missing its
     tenant filter returns correct-looking results in development, because dev data holds one tenant,
     and MongoDB has no row-level security to catch it.
   - **The diff adds pagination → check it is not `skip`/`OFFSET` at depth.** Cost is O(offset): fine
     on page 1 forever, quietly linear until someone asks for page 500 in production. No error, just a
     growing tail. Keyset pagination (`WHERE id > last_seen`) stays flat.

   > These three assume MongoDB, `tenant_id` and `db_read`, so they are project-specific content in a
   > global skill. Moving them to a project-level skill was considered and rejected: it needs a
   > cross-file reference that can go stale — one was created and had to be repaired inside this very
   > file — and whether a same-named skill at project scope overrides one at user scope is not
   > verified. The mechanical trigger above achieves the same isolation with neither risk.

5. **Independent review.**

   **Dispatch qua the engine, không tự chọn reviewer.** Xây ship-pack như dưới, rồi
   gọi **`Skill(znf:review)`** với `BASE=<base>` và ship-pack làm context. Engine tự
   chọn tier (T1 solo / T2 fan-out / T3 adversarial) theo kích thước + shared-touch —
   kể cả case diff lớn: engine bundle/adversarial thay vì ship tự "báo split & dừng".
   Engine trả `findings[]` (schema `znf:review/_shared/finding-schema.md`) + `shippable`.
   CRITICAL/HIGH vào fix-loop; MEDIUM/LOW lên board. `## Deferred` của ship-pack vẫn do
   engine chuyển cho reviewer một-dòng-mỗi-mục như cũ.

   **Build the ship-pack** — one file, so the reviewer reads it in a single call and the diff never
   lands in your context. Write it to `${TMPDIR:-/tmp}/ship-pack-<fp10>.md` (scratch, so it can never
   be committed):

   ```
   ## Intent      the plan file path if this came from /cook; the confirmed root cause if
                  from /fix or /hotfix; the user's stated goal otherwise
   ## Diff        git log --oneline <base>..HEAD
                  git diff --stat   <base>..HEAD
                  git diff -U10     <base>..HEAD
   ## Verified    what steps 2-4 actually produced — FACTS, never a verdict:
                    the commands run and their real output (test counts, lint result)
                    which test files ran, by name
                    which changed behaviour NO test touches
   ## Ground      db_read doc <each-real-collection-the-diff-touches>
                  db_read sql 'DESCRIBE <each-real-table-it-touches>'
   ## Deferred    every `minor (deferred)` line from the SDD ledger, verbatim:
                    rg -n 'minor \(deferred\)' .znf/sdd/*/progress.md
                  omit this block entirely when there is no ledger (/fix, /hotfix)
   ```

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

   Model scaling, reviewer count, and CRITICAL/HIGH-vs-MEDIUM/LOW routing are now the engine's
   decisions (`znf:review`) — ship no longer picks a model or a reviewer count itself.

6. **Deploy order** (multi-service only): schema/migration → backend → subscriber → frontend;
   subscriber before publisher for breaking pub/sub changes.

## Start the agents before the inline work

The inline work — `pm run lint`, `pm run build`, `db_read` — **blocks this loop while it runs**, so
starting it first idles every agent behind it. But the agents are not all startable at once: two need
only the diff, and one is genuinely downstream.

```
1. log in to the app          inline, blocking — the verifier cannot authenticate itself
2. ONE message dispatching:   the gate's per-repo sweeps  ‖  ui-verifier
3. inline, while they work:   pm run lint · pm run build · the three data checks
4. collect 2's reports BY NAME
5. THEN invoke Skill(znf:review) — it cannot start earlier, see below
6. read the board
```

Step 2 must be a **single message**. Separate messages run the agents in sequence and buy nothing.

**Why the reviewer is last and stays last.** Its ship-pack's `## Verified` block is *what steps 2-4
actually produced* — the real command output, which test files ran by name, and which changed
behaviour no test touches. Those facts do not exist until the inline checks have run. Dispatching it
earlier means building that block from expectation instead of output, which is the exact substitution
the block exists to prevent: a reviewer that cannot see what went untested approves code that is
syntactically fine and unverified. Concurrency is not worth buying with that.

The win is still where the time actually goes: the gate's eight per-repo sweeps are the slowest thing
in this gate, and they now run underneath lint and build instead of after them.

**One exclusive resource: the browser.** The Playwright instance is **shared and single** — never two
browser-driving agents at once, and the main session must not touch Playwright while `ui-verifier`
runs. It parallelises fine against the gate sweeps (different resources) — the reviewer is sequenced
after it for the separate reason above, not because of the browser. What it never parallelises against
is **itself**: a multi-screen change is one verifier covering several screens, not several verifiers.

**Two costs, both real:**

- **A failing check voids the concurrent work.** Any fix changes `fp`, so every stamp taken in that
  round is VOID and the loop re-runs them. Concurrency pays off on the pass path — which is the
  common one — and is wasted on the fail path. Worth it, not free.
- **Lost reports multiply.** 3 of 5 dispatches in one measured session finished without their report
  arriving (`CLAUDE.md §3`). With four out at once, expecting all four back unprompted is optimistic.
  **Ask each by name.** A report that never came makes this gate **incomplete** — never write ✅ for a
  check whose agent went quiet, because silence and a clean result are indistinguishable from here.

## The fix loop — it wraps every check, not just the review

**Any check failing enters the same loop**, whether it was lint, the gate, behaviour, or the review.
This used to live inside step 5, which meant a behaviour fix was never re-reviewed and a lint fix had
no defined re-verification at all — even though both are code changes.

```
round R = 1..2:
  a. fix ALL open findings in ONE wave — never one fixer per finding; each rebuilds context
     and re-runs the suite, and a real session's per-finding fix wave cost more than every
     task before it combined.
  b. recompute `fp`. It changed, so every check stamped with the old one is VOID. Re-run:
       step 2  always — code changed
       step 3  if a shape, payload, endpoint or queue changed
       step 4  if behaviour could differ, which for a code fix is the default
       step 5  scoped re-review — a fresh reviewer gets the findings list verbatim plus ONLY
               the fix diff (git diff <the-head-the-last-review-saw>..HEAD), verdicts each
               finding ADDRESSED / NOT ADDRESSED, and flags new breakage inside the fix diff
               only. Out-of-scope observations go on the board; they never extend the loop.
     No real output, no progress. A step you did not re-run stays ❌.
  c. all closed → step 7. Anything open → next round.
still open after round 2 -> STOP. Do not commit. Report to the user: the finding, what was
tried, and your own assessment of whether it is load-bearing.
```

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

## Step 7: Record the outcome, then commit

**Append one line before anything else** — this is what keeps the gate from becoming a CAB:

```bash
printf '%s\t%s\t%s\t%s\t%s\n' "$(date -u +%FT%TZ)" "$(basename "$PWD")" "$(fp)" \
  "SHIPPED|BLOCKED" "<step that blocked, or -->" >> ~/.cache/claude-ship-gate.tsv
```

Then read it back: `tail -30 ~/.cache/claude-ship-gate.tsv`. **If this gate has run many times and
never once blocked, say so to the user plainly.** That is the measured signature of an approval board
that approves everything — and the honest conclusion would be that these checks are costing time
without filtering anything, not that the work has been flawless.

## Output

```
Verified at fingerprint = <fp10>

✅/❌ Lint                (<fp10>)  <command run>
✅/❌ Build / typecheck    (<fp10>)  <command run>
✅/❌ Contract gate        (<fp10>)  /gate: <N repos impacted, or clean>
✅/❌ Behaviour verified   (<fp10>)  <N tests passed — or what /run showed>
      look: <ui-verifier verdict + the overflow numbers — or "nothing renders in this diff">
      data checks: <which of the project-specific ones ran; which the diff could not trigger>
✅/❌ Independent review   (<fp10>)  round <R>: <N CRITICAL/HIGH → addressed> · diff <N> LOC
      not blocking: <MEDIUM/LOW findings, plus any out-of-scope observations>
      deferred from the ledger: <N minor · which the reviewer called must-fix-before-merge,
                                 or "no ledger" for a /fix or /hotfix run>
Deploy order: ...
Gate log: <SHIPPED|BLOCKED> · blocked <N> of last <M> runs

Shippable: YES only if every fingerprint above == the current one
```

**Every ✅ carries the fingerprint it was earned at.** If any differs from the current one, that check
ran against a tree which no longer exists and the answer is NO — re-run it. This is mechanical, so the
user can check it without trusting the summary.

**Write the board to a file, then the caller `cat`s it.** Do not hand it back as prose for the caller
to retype:

```bash
BOARD="${TMPDIR:-/tmp}/ship-board-$(fp).md"     # scratch, so it can never be committed
cat > "$BOARD" <<'EOF'
<the board above, filled in>
EOF
cat "$BOARD"                                     # and report this path to the caller
```

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

Only call it shippable when every applicable line is ✅ **at the current fingerprint**, each backed by
output you actually saw. If anything was skipped, say so explicitly.

**On all-green you commit and push to the FEATURE branch** — same branch name across repos, clear
message, following the repo's existing convention (infer it from recent `git log --oneline` and branch
names if CLAUDE.md doesn't state it; don't invent a style). House rule #7 authorises this without
asking: pushing a feature branch deploys nothing.

**Do NOT create the PR** — that is the user's review decision. Report the pushed branch and the
suggested PR target. **NEVER push to or merge into a deploy/protected branch**; the project's
CLAUDE.md or `.claude/deploy-branches` lists them, and the git-guard hook enforces it. If anything is
❌, do not commit — fix first.
