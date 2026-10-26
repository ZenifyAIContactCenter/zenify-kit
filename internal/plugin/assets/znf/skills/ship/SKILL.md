---
name: ship
description: Pre-ship gate. Use when work is complete and about to be committed — lint/build, contract gate, behavioural verification and independent review with one fix loop, then commit, push and open the PR (never merges). Invoked unconditionally by /cook, /fix and /hotfix.
allowed-tools: Bash(git *) Bash(pm *) Bash(zenify db-read *) Bash(rg *) Bash(printf *) Bash(cat *) Bash(tail *) Bash(wc *) Read Grep Agent
---

Tool names for each action: `znf:_shared/harness-tools`.

## Fingerprint the tree, not HEAD

```bash
git status --short && git diff --name-only
fp() { { git rev-parse HEAD; git diff HEAD; git status --porcelain -uall; } \
         | git hash-object --stdin | cut -c1-10; }
fp
```

Stamp each check with its `fp`; step 7 needs all current.

**Plan has a `## Stack` block** → steps 2–7 run per `references/stacked-ship.md`; below is the one-PR flow.

## Steps

1. **Scope**: list the changed areas from the changed files.

2. **Static checks**: per changed area run lint and build/typecheck, reporting the real output — no pass without it. **Never assume `npm`**: `pm run lint` / `pm run build`. Then `zenify rules lint ~/.zenify/knowledge/.config/rules` (kit repo: `zenify rules lint --include-go` too) — one non-English agent-read line fails it.

3. **Contract gate**: anything shared touched (collection, endpoint, queue, pub/sub channel) → run the project's contract gate and report the repos it found. Never judge a change "local".

4. **Behavioural verification**: tests if any exist, else **`Skill(znf:run)`**, which reports the URL `znf:ui-verifier` needs — "it builds" is not "it works". **A suite that ran nothing is not a pass**: report the count.

   ```bash
   git diff --name-only HEAD | rg -c '\.(tsx|jsx|vue|svelte|css|scss|less)$|components?/|pages?/|views?/'
   ```

   Non-zero → dispatch **`znf:ui-verifier`**; **do not drive the browser yourself**. Zero (literally, never a judgement) → "nothing renders in this diff". **A non-zero with no verdict = BLOCKED (❌, Shippable NO).** Setup (flag ON, seed, FE+BE up) is work you perform, never a skip; cannot → BLOCKED, naming what is missing. `ZNF_AUTONOMOUS=1`: `references/autonomous-ui.md`.

   - **`.znf/visual/routes.json` present → `zenify visual check --repo <path> --port <P>` FIRST**, a hard gate on non-zero exit; then still dispatch `znf:ui-verifier`. `.znf/e2e/` → `zenify e2e lint`.
   - The verifier logs itself in (`zenify e2e login --role <r>`, also on `visual check`: a role the screen needs, never admin by default); require **both** a screenshot **and** a `getBoundingClientRect` of the changed element against its container. `--repo` = this worktree; it records via `zenify ui-verify record`. Distinct session name per verifier.
   - **A negative result that lets you proceed is not evidence** until a second mechanism agrees.

   **Three data checks, triggered mechanically** — trigger commands, tenant and pagination rules in
   `references/false-green-and-data.md`; run them and say which the diff could not trigger. **A DB
   query added or changed → `Skill(znf:explain-plan)`** (`COLLSCAN` / `Seq Scan` / MySQL `type=ALL` / scan-ratio): an unwaived **BLOCKING** finding
   (`// znf:db-perf-ok: <reason>`) means **ship does not complete**; ADVISORY prints under `## DB-Perf`.

5. **Independent review** — never pick the reviewer. First `BASE=$(git merge-base "$(zenify wt base)" HEAD)`. Write the pack to `${TMPDIR:-/tmp}/ship-pack-<fp10>.md`, then **`Skill(znf:review)`** with `BASE=$BASE` and the pack: it picks the tier and returns `findings[]` (`znf:review/_shared/finding-schema.md`) plus `shippable`. CRITICAL/HIGH enter the fix loop, MEDIUM/LOW and each `Declined:` line the board.

   ```
   ## Intent      plan path (/cook) · root cause + Loop:/Regression: lines (/fix, /hotfix) · else the user's goal
   ## Diff        git log --oneline $BASE..HEAD · git diff --stat/-U10 $BASE..HEAD
   ## Verified    FACTS from steps 2-4, never a verdict: commands + real output, the test
                  files that ran by name, the changed behaviour NO test touches
   ## Ground      zenify db-read doc <collection> · zenify db-read sql 'DESCRIBE <table>'
   ## Deferred    every `minor (deferred)` and `parked` line from the SDD ledger, verbatim:
                    rg -n 'minor \(deferred\)|parked —' .znf/sdd/*/progress.md
                  omit with no ledger (`references/ship-pack-rationale.md`)
   ```

6. **Deploy order** (multi-service): schema/migration → backend → subscriber → frontend; subscriber first for a breaking pub/sub change.

**Start the agents first**: dispatch sweeps and `znf:ui-verifier` in **ONE message**, do lint/build/data checks inline, collect each report **by name**, review last. A missing report = gate **incomplete**.

## The fix loop — it wraps every check, not just the review

**Any failing check enters this loop.** Fix ALL open findings in ONE wave, never one fixer per finding, then recompute `fp`: any check stamped with the old one is VOID.

- Re-run step 2 always; step 3 if a shape, payload, endpoint or queue changed; step 4 if behaviour could differ (the default). A step not re-run stays ❌.
- Step 5 re-runs **scoped**: findings verbatim plus ONLY the fix diff (`git diff <the-head-the-last-review-saw>..HEAD`), each verdicted ADDRESSED / NOT ADDRESSED.
- **Still open after round 2: STOP, do not commit**; report the finding, what was tried, and whether it is load-bearing.

## Step 7: Record the outcome, then commit

**Append one line first:**

```bash
printf '%s\t%s\t%s\t%s\t%s\n' "$(date -u +%FT%TZ)" "$(basename "$PWD")" "$(fp)" \
  "SHIPPED|BLOCKED" "<step that blocked, or -->" >> ~/.cache/claude-ship-gate.tsv
```

Read it back (`tail -30`); **if never blocked, say so.** Trailer `Spec: <spec path>` when a spec exists. Then fill the board (template: `references/gate-evidence.md`) — a `✅/❌ <check> (<fp10>) <evidence>` line for lint, build, contract gate, behaviour and review, plus `Deploy order:`, `Gate log: <SHIPPED|BLOCKED> · blocked <N> of last <M>`, `Shippable: YES only if every fingerprint above == the current one`.

Fill the `look:`/review sub-lines per the template. **Before concluding: `zenify ui-verify check --repo <path> --base "$(git merge-base "$(zenify wt base)" HEAD)"` — non-zero → append `❌ BLOCKED` and Shippable NO (`ZNF_AUTONOMOUS=1` exit 8 = the await stop: end the session, no BLOCKED line); a waiver appends `waived: <reason>`.**

**Write the board to `${TMPDIR:-/tmp}/ship-board-$(fp).md`**, `cat` it, report path.

**7b.** Before pushing, write the risk-metadata note-commit (`_Blast-radius:`/`_DB:`/`_Rollback:` from the Brief spec's tags, else blank):

    zenify release-note --slug "<slug>" --note "<one-line description>" \
      --blast "<Brief _Blast-radius>" --db "<Brief _DB>" --rollback "<Brief _Rollback>" \
      --spec "<spec path if any>"

Then push and `zenify docs sync` (fails open).

**On all-green, commit and push the FEATURE branch** — same branch name across repos, message in the repo's convention. **Then open the PR yourself and stop** (`--watch` → `Skill(znf:ci-watch)`). `features: github.issues on` in the session → `references/github-issues.md` first. `gh pr create`, one per repo onto its base (protected is fine, a PR deploys nothing), title and body per `references/pr-message-language.md`. Report each URL.

**Merging is the user's deploy decision — NEVER merge, including a PR you opened**: no `gh pr merge`, no merge or push into a deploy/protected branch (listed in the project's CLAUDE.md or `.claude/deploy-branches`). Anything ❌ → fix first, do not commit.

## References

- `references/gate-evidence.md` — board template; why the gate exists.
- `references/ui-verification-notes.md` — verifier stalls or logs out.
- `references/false-green-and-data.md` — the three data checks.
- `references/ship-pack-rationale.md`
- `references/pr-message-language.md` — PR language.
- `references/stacked-ship.md` — per-layer review.
- `references/autonomous-ui.md` — step 4 under `ZNF_AUTONOMOUS=1`.
- `references/github-issues.md` — `github.issues` on.
