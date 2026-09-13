---
name: ship
description: Pre-ship gate. Use when work is complete and about to be committed — runs lint/build on the changed areas, the cross-service contract gate, behavioural verification, and an independent review, with one fix-and-re-verify loop over all of them, then commits and pushes the feature branch and opens the PR (never merges). Invoked unconditionally by /cook, /fix and /hotfix.
allowed-tools: Bash(git *) Bash(pm *) Bash(db_read *) Bash(rg *) Bash(printf *) Bash(cat *) Bash(tail *) Bash(wc *) Read Grep Agent
---

## What this gate is actually for

> Why: see `references/gate-evidence.md` — which parts carry weight.

## Working tree — fingerprint the tree, not HEAD

> Why: see `references/gate-evidence.md` — why HEAD is not what you tested.

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

   - Run the agent-read language gate on the distributed team rules:
     `zenify rules lint ~/.zenify/knowledge/.config/rules`. The rules that reach teammates must be
     English so they stay portable and reviewable; any Vietnamese in a rule `.md` (outside a
     code-fence, inline-code, or a `<!-- znf:allow-lang -->` line) fails the gate — fix it before
     opening the PR. When the change is to the kit repo itself, also run the full-tree gate over its
     own skill assets and Go source: `zenify rules lint --include-go`. Agent-read comments, error
     values and test assertions must be English; only human-facing CLI output kept in Vietnamese is
     allowed, and each such line must carry a `//znf:allow-lang` marker.

3. **Contract gate**: if the change touched anything shared across services — a DB collection, an
   HTTP endpoint between services, a queue, a pub/sub channel — run the project's contract gate,
   where it defines one, and report which repos it found. Do not decide by judgement that a change
   "looks local"; the gate answers that.

4. **Behavioural verification**: confirm the change actually works. Tests if they exist; otherwise
   **`Skill(znf:run)`** and observe the real code path — "it builds" is not "it works". Invoke it as a
   tool: it reads the port this worktree was allocated rather than hunting for a free one, and the
   URL it reports is what `znf:ui-verifier` needs. A `Skill(znf:run)` line is checkable; "I ran the app" is
   the shape that dissolves into unnamed `Bash` calls.

   > Why: see `references/ui-verification-notes.md` — why this pass is authoritative.

   **Trigger it mechanically, not by judgement:**

   ```bash
   git diff --name-only HEAD | rg -c '\.(tsx|jsx|vue|svelte|css|scss|less)$|components?/|pages?/|views?/'
   ```

   Non-zero → dispatch **`znf:ui-verifier`** (the plugin agent) and **do not drive the
   browser yourself**. Zero → write "nothing renders in this diff" on the board and move on.

   > Why: see `references/ui-verification-notes.md` — why the agent exists.

   **If the target repo has `.znf/visual/routes.json`, run `zenify visual check --repo <path> --port <P>`
   FIRST** — golden-diff catches visual regression in regions *unrelated* to the diff, which the
   single-element `znf:ui-verifier` measurement cannot. A non-zero exit is a hard gate: fix before shipping. This is the local half; CI runs
   the same check as a backstop. Then still dispatch `znf:ui-verifier` for the changed element's overflow
   measurement — the two are complementary, not substitutes.

   If the repo has `.znf/e2e/`, run `zenify e2e lint` — it blocks a shallow journey before the PR is opened.

   **Log in yourself first, then hand the live session over.** Neither verifier can get past a login:
   they have the eight ordinary browser tools and **not** `browser_run_code_unsafe`, and their attempt to
   read credentials is classifier-blocked. So the main session logs in — `browser_snapshot` for the refs,
   then `browser_type` into the fields and `browser_click` the button, with values read from the
   workspace `settings.local.json` — and only then dispatches the verifier onto the already-authenticated
   browser. If a `browser_type` carrying a password is refused
   once with *"Stage 2 classifier error — usually transient, retrying often succeeds"*, that means what it
   says: retry, do not start building a way around it. Tell the verifier **not** to clear
   `localStorage` or cookies — one logged itself out mid-run.

   > Why: see `references/ui-verification-notes.md` — the incidents this avoids.

   Tell it the dev URL,
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
   delegating them.

   > Why: see `references/false-green-and-data.md` — why delegating the summary is wrong.

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

   > Why: see `references/false-green-and-data.md` — the CI-provider `null` incident.

   **Three data checks — cheap, and almost never run.** Each catches a class that
   development-sized data hides completely, so passing tests say nothing about them.

   **Trigger them mechanically, not by remembering which project you are in:**

   ```bash
   command -v db_read >/dev/null || echo "no db_read on PATH — these three do not apply here"
   git diff HEAD | rg -c '\.find\(|\.aggregate\(|\.skip\(|OFFSET|findOne\(|updateMany\('
   ```

   Both true → run them. Either false → say which and why, and move on.

   > Why: see `references/false-green-and-data.md` — why a command, not a note.

   Report which you ran and which the diff could not trigger.

   - **The diff adds or changes a DB query → run the two-tier DB-perf gate** (`COLLSCAN` /
     `Seq Scan` on a large collection or table is one of the findings). Delegate the full
     size-aware rubric to **`Skill(znf:explain-plan)`** — it runs `zenify db-perf` plus
     `db_read eval '…explain("executionStats")'` / `EXPLAIN ANALYZE` per site. An index existing
     does not mean it is used (non-selective field, wrong compound-index column order, `$in`/`$or`).
     A **BLOCKING** finding that is not waived (`// znf:db-perf-ok: <reason>` on the query line)
     means **ship does not complete** — list each one with its fix. ADVISORY findings print under
     `## DB-Perf` and do not block.
   - **The diff touches a query on a tenant-scoped collection → assert the negative.** Query tenant
     B's context against a row known to belong to tenant A and assert **zero rows**. One test case,
     and it exercises the whole mandatory-filter path. Nothing substitutes for it: a query missing its
     tenant filter returns correct-looking results in development, because dev data holds one tenant,
     and MongoDB has no row-level security to catch it.
   - **The diff adds pagination → check it is not `skip`/`OFFSET` at depth.** Cost is O(offset): fine
     on page 1 forever, quietly linear until someone asks for page 500 in production. No error, just a
     growing tail. Keyset pagination (`WHERE id > last_seen`) stays flat.

   > Why: see `references/false-green-and-data.md` — why not a project-level skill.

5. **Independent review.**

   **Dispatch through the engine, don't pick the reviewer yourself.** Build the ship-pack as below, then
   call **`Skill(znf:review)`** with `BASE=<base>` and the ship-pack as context. The engine itself
   picks the tier (T1 solo / T2 fan-out / T3 adversarial) by size + shared-touch —
   including the large-diff case: the engine bundles/goes adversarial instead of ship itself "reporting split & stop".
   The engine returns `findings[]` (schema `znf:review/_shared/finding-schema.md`) + `shippable`.
   CRITICAL/HIGH go into the fix-loop; MEDIUM/LOW go on the board. The ship-pack's `## Deferred` still gets
   passed to the reviewer one-line-per-item by the engine, as before.

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

   > Why: see `references/ship-pack-rationale.md` — why each field exists.

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
2. ONE message dispatching:   the gate's per-repo sweeps  ‖  znf:ui-verifier
3. inline, while they work:   pm run lint · pm run build · the three data checks
4. collect 2's reports BY NAME
5. THEN invoke Skill(znf:review) — it cannot start earlier, see below
6. read the board
```

Step 2 must be a **single message**. Separate messages run the agents in sequence and buy nothing.

> Why: see `references/ship-pack-rationale.md` — why the reviewer is last.

**One exclusive resource: the browser.** The Playwright instance is **shared and single** — never two
browser-driving agents at once, and the main session must not touch Playwright while `znf:ui-verifier`
runs. What it never parallelises against
is **itself**: a multi-screen change is one verifier covering several screens, not several verifiers.

> Why: see `references/ship-pack-rationale.md` — why the browser doesn't clash with the sweeps.

**Two costs, both real:**

> Why: see `references/gate-evidence.md` — the two costs, in full.

- **Lost reports multiply. Ask each by name.** A report that never came makes this gate **incomplete** —
  never write ✅ for a check whose agent went quiet, because silence and a clean result are
  indistinguishable from here.

## The fix loop — it wraps every check, not just the review

**Any check failing enters the same loop**, whether it was lint, the gate, behaviour, or the review.

> Why: see `references/gate-evidence.md` — why the loop wraps every check.

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

> Why: see `references/gate-evidence.md` — why two rounds, not five.

## Step 7: Record the outcome, then commit

**Append one line before anything else** — this is what keeps the gate from becoming a CAB:

```bash
printf '%s\t%s\t%s\t%s\t%s\n' "$(date -u +%FT%TZ)" "$(basename "$PWD")" "$(fp)" \
  "SHIPPED|BLOCKED" "<step that blocked, or -->" >> ~/.cache/claude-ship-gate.tsv
```

Then read it back: `tail -30 ~/.cache/claude-ship-gate.tsv`. **If this gate has run many times and
never once blocked, say so to the user plainly.**

> Why: see `references/gate-evidence.md` — what a 0%-block rate actually signals.

## Output

```
Verified at fingerprint = <fp10>

✅/❌ Lint                (<fp10>)  <command run>
✅/❌ Build / typecheck    (<fp10>)  <command run>
✅/❌ Contract gate        (<fp10>)  /gate: <N repos impacted, or clean>
✅/❌ Behaviour verified   (<fp10>)  <N tests passed — or what /run showed>
      look: <znf:ui-verifier verdict + the overflow numbers — or "nothing renders in this diff">
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

Only call it shippable when every applicable line is ✅ **at the current fingerprint**, each backed by
output you actually saw. If anything was skipped, say so explicitly.

> Why: see `references/gate-evidence.md` — why the board must be `cat`ed.

**7b. Write the release-note + update unreleased.md (release-log-at-ship).**

Before pushing the feature branch, write a note-commit carrying risk-metadata so the release doc already has
this change (no need to remember to log it by hand). Take the values from the ship-pack's `## Intent` + the Brief spec (if
`/cook` has a spec): `_Blast-radius:`/`_DB:`/`_Rollback:` copied from the three Brief tags; no spec →
leave blank, the command fills in the default itself (`unknown` / `N/A` / `revert PR`).

    zenify release-note --slug "<slug>" --note "<one-line description>" \
      --blast "<Brief _Blast-radius>" --db "<Brief _DB>" --rollback "<Brief _Rollback>" \
      --spec "<spec path if any>"

Then push the feature branch (note-commit comes along). After pushing, regenerate the view right away:

    zenify release-report --unreleased --workspace "<workspace root>"
    zenify docs sync   # push unreleased.md into the knowledge store (proactively, don't wait for the Stop-hook)

Fail-open: both commands return clean on their own if they error — they do NOT block `/ship`. `unreleased.md` is a view
derived-from-git; the change just shipped appears once merged into staging (git is the source of truth at settle time).

**Encouraged, not enforced — the `Spec:` commit trailer.** When a change implements a spec, add a
trailer line `Spec: specs/<repo>/<date>-<topic>-design.md` to the commit (or squash-merge) body.

> Why: see `references/ship-pack-rationale.md` — what the trailer buys `zenify spec status`.

**On all-green you commit and push to the FEATURE branch** — same branch name across repos, clear
message, following the repo's existing convention (infer it from recent `git log --oneline` and branch
names if CLAUDE.md doesn't state it; don't invent a style).

> Why: see `references/gate-evidence.md` — the house rule that authorises this.

**Open the PR yourself, then stop** — the release report is derived **per-PR**, so a well-formed
PR is what keeps the changelog clean. After pushing, run `gh pr create` with a clean conventional
title and a structured body following the repo's PR template/convention (one PR per repo, same
branch, targeting the repo's base — a deploy/protected base is fine here: opening a PR against it
deploys nothing). Report each PR URL and stop.

**Merging is the user's deploy decision — NEVER merge, including a PR you opened.** No `gh pr merge`,
no merge into a deploy/protected branch. The git-guard hook blocks the local-git path, but
`gh pr merge` is server-side, so this is a behavioural rule too. **NEVER push to or merge into a
deploy/protected branch** (the project's CLAUDE.md or `.claude/deploy-branches` lists them). If
anything is ❌, do not commit — fix first.

## References

Materialized at `~/.claude/skills/znf/skills/ship/references/`. Read a file only when its trigger fires.

- `references/gate-evidence.md` — read when a clean review tempts you to skip step 4, or someone asks why the board exists.
- `references/ui-verification-notes.md` — read when the UI verifier stalls, logs itself out, or you doubt why the check runs here and not in `/cook`.
- `references/false-green-and-data.md` — read when a negative result ("no match", "OK", 0 tests) is about to let you proceed, or you wonder why the three data checks are global.
- `references/ship-pack-rationale.md` — read when you are tempted to write a verdict into `## Verified`, drop `## Deferred`, or dispatch the reviewer early.
