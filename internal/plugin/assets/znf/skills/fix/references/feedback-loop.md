<!-- Adapted from mattpocock/skills diagnosing-bugs (MIT). Read when: building the Step 0 loop, or Step 2/5/6 need it. -->

# The feedback loop

Step 0 ends with **one named command, already run**, whose output is red on exactly the symptom the
user described. Every later step re-runs it: Step 2 confirms the cause with it, Step 5 turns it into
a regression test, Step 6 watches it go green.

## Redact first

Before a command, its output, or any artifact (log, payload, HAR, dump) enters the conversation or a
file, replace every secret — token, password, API key, cookie, connection string — with
`<REDACTED>`. The loop reads credentials from environment variables (`$API_TOKEN`), never from a
pasted value.

## The ladder

Try the rungs top-down; stop at the first one that goes red on the right symptom.

1. **Failing test** at the seam that touches the bug.
2. **`curl`** against the worktree's own port (the one `wt` allocated).
3. **CLI with a fixture** — the real entry point fed a saved input.
4. **Replay a captured payload** — the request, job or message that failed, redacted.
5. **Throwaway harness** — a short script that calls the failing function directly.
6. **Property / fuzz loop** — generate inputs until one fails.
7. **`git bisect run <cmd>`** — when it worked before and you have a command that tells good from bad.
8. **Old vs new** — same input against the good build and the bad build, diff the outputs.
9. **Numbered manual steps** — last resort: steps for the user to perform by hand, each returning
   one `KEY=value` line you can read back. No script.

Read-only rungs — reading logs, running an existing failing test, `curl` against an environment that
is already running — need no worktree. A rung that writes a file or starts a server needs the
worktree first.

## Tighten the loop

- **Fast** — seconds, not minutes. Narrow to the one test, the one request, the smallest fixture.
- **Sharp** — one assertion on the exact symptom, not "some exception was thrown".
- **Stable** — same result every run. A loop that is red one time in three proves nothing.

## Flaky bugs

Raise the reproduction rate before you guess: repeat N times in a loop, force the timing or the
concurrency (sleep, parallel runs, a smaller pool), pin the seed and the clock. Guess only once the
loop is red reliably.

## Done when

- [ ] It goes red.
- [ ] It is red on the symptom the user described, not a neighbouring error.
- [ ] It is stable across repeated runs.
- [ ] It is fast enough to re-run after every change.
- [ ] You can run it yourself, without the user.

## Debug logs

Every temporary log line carries a unique prefix `[DEBUG-xxxx]` (four hex characters, one per
session), so removal is a search, not a memory. Before reporting, both must return nothing:

```bash
rg '\[DEBUG-' <changed files>
git diff <base> | rg '\[DEBUG-'
```

## Performance bugs

Measure a **baseline** as a number before changing anything; the loop is a command that prints that
number. Then bisect — by commit, by input size, or by stage — until the number moves.

## No loop

If the whole ladder yields no red command: stop. List what you tried and what each rung printed,
then ask for one of (a) access to an environment that reproduces it, (b) a redacted artifact from the
failure (log, payload, HAR), (c) permission to add temporary `[DEBUG-xxxx]` logging where it fails.
Do not move on to hypotheses without a loop.
