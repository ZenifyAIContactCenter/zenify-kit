---
name: run
description: Launch the app and produce real output from the real code path, so a change can be verified rather than asserted. Use when a change is behavioural and there are no tests covering it, before claiming it works, and before dispatching ui-verifier (which needs the URL this produces). Reads the port the worktree was allocated instead of hunting for a free one.
allowed-tools: Read Grep Glob Bash(git *) Bash(rg *) Bash(herdr *) Bash(hcall *) Bash(cat *) Bash(nc *) Bash(curl *) Bash(node *) Bash(tail *) Bash(grep *)
---

`CLAUDE.md` rule #3: *"When there are no tests, produce output from the real code path and show
it."* This skill is how. It does not judge the output — it makes output exist.

**This skill existed as a name before it existed as a file.** Eight places referenced `/run`
— including rule #3 itself and a `WORKFLOW.md` row claiming *"Agent-invocable, so skills can rely
on it"* — while `skills/run/` did not exist. The visible consequence: with nothing owning "which
port does this app use", agents improvised a port hunt, which silently discards the port `wt`
allocated and puts the app somewhere `ui-verifier` is not looking.

## Step 1: The port is already decided — read it, never hunt for it

`wt` allocated one port per worktree and recorded it. Hunting for a free port throws that away
and lands the app somewhere nothing else expects.

```bash
PORT=$(git config --get wt.port)      # inside a wt worktree
```

Empty means this is not a `wt` worktree — the main checkout, or a repo where `wt` cannot run.
Say which, and use the project's documented default; do not invent one.

### Then stop, if something is already serving it

```bash
lsof -nP -iTCP:"$PORT" -sTCP:LISTEN            # never `nc` — see Step 5
```

Something there → do **not** start a second one. A second Vite prints `Port 3338 is in use, trying
another one...` and comes up on 3339, after which every URL you report is wrong and `ui-verifier`
exercises the *first* server — the one without your change.

**But reusing it depends on whose code it is running, and that has to be checked.** Ask the process,
not the port:

```bash
PID=$(lsof -nP -iTCP:"$PORT" -sTCP:LISTEN -t | head -1)
lsof -a -p "$PID" -d cwd -Fn | grep '^n' | sed 's/^n//'      # `-a`, or lsof ORs the selectors
```

| That cwd is | Then |
|---|---|
| the repo's **main checkout** | reuse it. Unmodified baseline, identical for every task |
| **your own worktree** | reuse it. It is your code |
| **another worktree** | **do not reuse.** That is another task's uncommitted code, and testing against it passes or fails for reasons that have nothing to do with your change. Say whose it is and stop |

The third row is the one that bites, because reuse looks free and the contamination is invisible:
your change verified green against a server running someone else's half-finished edit.

**A task only needs a local server for a repo it actually touched.** An untouched repo has the same
code for every task, so it does not need a per-task copy — and where the project points its
frontend at a deployed environment by default, it does not need a local copy at all. Read the
frontend's env file before starting a backend: if it points at staging, a local backend on the
default port is serving nobody. Measured, on this skill's own demo run — `be` and `hub` were started
locally while the frontend's `.env` pointed both of them at `*-staging`, so both ran for nothing.

## Step 2: How the port reaches the app is per-repo, and must be read from code

**`portEnv` in `.claude/worktree.json` is a claim about the app, and it can be wrong.** `wt`
writes `<portEnv>=<port>` into the worktree's env file whether or not anything reads that
variable. Three shapes, each met in a real repo:

| Shape | How to recognise it | What to do |
|---|---|---|
| the code reads the env var directly | `process.env.<NAME>` at the listen site | nothing; `wt` already wrote it into the env file |
| a config file reads the **env file** | a bundler config calling something like `loadEnv(...)` rather than `process.env` | nothing — but it works because `wt` wrote the *file*; `export`ing the variable would not |
| a config file with **no env path at all** | a config library reading `*.yml`/`*.json`, no `${}` interpolation, no env-variable mapping file | `portEnv` **cannot work**; write a gitignored per-worktree override |

**Which shape a repo is, is a project fact — look it up, never carry it between projects.** It
belongs in that repo's `CLAUDE.md`; this table only says which shapes exist and what each implies.
An earlier version listed repos by name and asserted one of them read `PORT` via Next. It was Vite,
the default was not the one stated, and the value arrived through the env *file* rather than the
environment — three wrong claims in one row, none of which mattered until they did. A named row is
exactly the thing that stops being true, quietly, while still reading as authority.

The third one is the trap, and it is invisible from config alone: hub's `.env` contains
`PORT=3002`, which *matches the hardcoded default*, so the setup looks wired when nothing reads
it. Every hub worktree then listens on 3002 — colliding with the main checkout and with every
other hub worktree.

**For that shape, write a gitignored per-worktree override — do not skip it and do not reach for an
env var.** node-config loads `local.EXT` after `default.EXT`
(`node_modules/config/lib/config.js:457`), and in hub `config/` is already gitignored, so the
override cannot leak into git or into a deployment. Nothing creates this file for you: `wt` writes
only the `portEnv` line into the env file, so **this step is `/run`'s job on every launch**, not a
one-off someone did once.

```bash
# write it when absent, or when it disagrees with the port wt allocated
printf 'service:\n  hub:\n    port: %s\n' "$(git config --get wt.port)" > config/local.yml
NODE_ENV=development node -e 'console.log(require("config").get("service.hub.port"))'
```

Then check it against `git status --porcelain` — if the override shows up as a change, it is **not**
gitignored in this repo and writing it would dirty the branch. Stop and say so instead.

**Confirm at the config layer *and* at the socket.** The config layer is where this class of bug
lives; the socket is where the claim lands. Verified once end-to-end: the override resolved to the
allocated port, the worktree's server took it, and the main checkout's kept its own — two instances
at once, impossible before.

**What not to do: teach the config library to read environment variables.** Adding a global
env-variable mapping makes every documented-but-inert name live in *all* environments at once,
including production, where a `.env` written to match a stale README may already set one. That is a
deployment change disguised as a dev-environment fix, and it is the user's call, not this skill's.

**A repo whose config directory is gitignored cannot run from a bare checkout.** Seed it the same
way `.env` is seeded — `wt`'s `copy` list takes directories (`cp -c -R`, `wt:410`). Without it the
worktree silently falls back to code defaults, which for hub means `mongodb://localhost:27017/test`
and a placeholder JWT secret: a failure that reads exactly like a broken change.

So before running an app whose port shape you have not read this session:

```bash
rg -n "listen\(|env\.PORT|process\.env\.[A-Z_]*PORT" -g '!node_modules' | head
```

Find what the code reads. Then make the allocated port reach *that*.

### Then wire the peers — before starting anything

```bash
wt wire            # --dry-run first if you want to see it
```

Getting the app's **own** port right is only half of it. The other half is where it looks for the
*other* services, and that is where the silent pass lives: a frontend worktree whose env still
points at the baseline is testing against the **unchanged** backend, and it goes green.

`wt wire` recomputes each declared peer variable from scratch — the peer's worktree port when a
worktree of **this slug** exists, otherwise the value the main checkout has. So it is idempotent,
it reverts to baseline when the peer is torn down, and a *different* task's worktree is never wired
in (that would verify your change against someone else's uncommitted edit). Declared per consumer
repo, keyed by env var rather than by repo, because one repo can serve several services:

```json
"peers": { "VITE_HUB_URL": { "repo": "…", "url": "http://localhost:{port}" } }
```

**Run it even when the task touches only one repo.** A worktree's env file is copied at creation
and frozen there, while the main checkout's moves on. Measured on a real worktree three days old:
it still carried `localhost:3001` / `localhost:3002` after the baseline had been changed to point
at a deployed environment — so it was aimed at whatever happened to be occupying those ports.
Re-syncing the baseline is the same command.

**Before the server starts, not after.** A bundler reads its env files at config time
(`loadEnv(...)`), so a wire that lands after the dev server booted changes nothing until a restart —
and the restart is the part nobody remembers.

## Step 3: The launch command comes from the project, not from memory

Read the `Commands` section of the repo's `CLAUDE.md`. **Stop when the recipe is ambiguous, not
merely when `CLAUDE.md` is absent** — that distinction cost this skill its first run, where it was
about to refuse a repo that had no `CLAUDE.md` and exactly one dev script. Refusing there is the
rule serving itself. Stop and say so when there are several plausible candidates, or none — and
note the missing recipe either way, so it gets written. A wrong launch command produces a failure
that looks like a broken change; an unambiguous one that happens to be undocumented does not.

Filling the gap belongs to `/onboard-project`, which is already told to record the launch command
and port for exactly this reason.

**"No `dev` script" does not mean no dev command.** Scripts are often named after the **app**
rather than the mode — in a monorepo, one entry per deployable — so searching for `dev`/`start:dev`
and concluding there is none is a mistake this skill made on its first run. Read the whole
`scripts` block:

```bash
node -e 'console.log(Object.keys(require("./package.json").scripts).join("\n"))'
```

`--debug` in that command also means the Node inspector binds its default 9229, which **is not
per-worktree** — a second watch server in another worktree collides there even when the HTTP port
is correct.

## Step 4: Run it in a pane, not in this session

A dev server is long-lived. Started from the session's Bash it either blocks the turn or is
orphaned, and its output lands nowhere anyone can look at again. When herdr is present, give it
a pane — **in a split below the agent, not in a new tab.**

**Reuse before creating, on both axes.** A pane already running this repo's server is the answer
to "where does it go"; a second pane for the same repo is how you end up with two servers and one
of them on a drifted port.

```bash
herdr pane layout --pane "$HERDR_PANE_ID"      # what is already in this tab
```

Then, in order:

1. **A pane for this service already exists** → send the command there, or nothing if Step 1 found
   it already serving.
2. **No service column yet** → split the agent's pane **right**, so the agent keeps its **full
   height**:
   ```bash
   herdr pane split --pane "$HERDR_PANE_ID" --direction right --ratio 0.66 --cwd "$PWD" --no-focus
   ```
3. **A column exists** → split the **bottom pane of the column** downward. Ratios that come out
   even, measured rather than derived at runtime:

   | Services in the column | Splits |
   |---|---|
   | 2 | `down 0.5` |
   | 3 | `down 0.34` then `down 0.5` |
   | 4 | `down 0.25`, `down 0.34`, `down 0.5` |

4. **Only past four** → `herdr tab create --label dev`. At 62 rows a fifth pane leaves each under
   13 rows, which is less than a stack trace.

Note `herdr pane split` rejects `--json`; it prints JSON regardless.

Label every pane after its repo — `herdr pane rename <pane_id> <repo-short-name>` — because the
branch name is identical across repos and the path is truncated in the border.

```bash
hcall pane.send_text "{\"pane_id\":\"<pane_id>\",\"text\":\"<dev command>\n\"}"
```

**The unit is a service, not a repo.** A monorepo runs several apps from one checkout, each on its
own port, so one repo can need three panes by itself and "how many repos does the task touch"
answers the wrong question. Count services you are actually starting.

**`wt` allocates one port per worktree — a real gap, not a convention.** A worktree running a
second app has no allocated port for it; that port has to be written by hand into the same
gitignored override the third shape above uses, chosen from the repo's declared `portRange`. Read
the repo's `CLAUDE.md` for which apps it runs and which key each takes its port from; do not assume
the app you know is the only one.

**Why the column and not a band across the bottom.** Both were built and measured at 203×62:

```
column (right)   agent 134x62   ·  three services 69x21, 69x21, 69x20   → 3 fit, no tab
band (bottom)    agent 203x47   ·  two services 102x15, 101x15          → 3rd needs a tab
```

The column wins on the axis that matters most and the one nobody counts: **the agent pane is what
you read continuously**, and 62 rows against 47 is a third more conversation on screen. The service
panes trade width (69 vs 102) for height (21 vs 15) and for all three fitting at once — and for a
dev log you read the tail, so height is what shows you a stack trace.

**The count that decides overflow is panes in the column, not repos in the task.** Panes-in-column
is state you can measure and it corrects itself when one is closed; repos-in-task is a guess made up
front that goes wrong the moment the task grows — and it is the wrong unit anyway, since one
monorepo can want three of these on its own.

This file has now had the layout wrong twice, in opposite directions, and both times from
over-generalising one measurement. First *"a tab rather than a split: a split leaves each dev pane
~62 columns"* — true of a **vertical** split (agent 121 | dev 62) and then applied to splitting in
general. Then a bottom band, which does give each pane full width but takes 15 rows off the agent
and caps the column at two. The measurement that settles it is the one above: build both, read the
geometry, prefer the layout that protects the pane you actually read.

`--no-focus` throughout, and afterwards `herdr workspace focus "$HERDR_WORKSPACE_ID"`
unconditionally — whether anything steals focus measured differently on two runs, and restoring
costs one call either way.

**Send the bare command. Never pipe a watch server through `tail` or `head`.** `tail` waits for
EOF, which a `--watch` process never reaches, so `npm run hub 2>&1 | tail -40` produces **no output
at all** — and then the readiness wait times out and the app looks broken while it is running fine.
Measured, on the first attempt at exactly this. If output volume is the worry, bound it by reading
fewer lines back (`pane.read --lines N`), not by filtering at the source.

**Without herdr** — a plain terminal, or a subagent — run it detached and read the log:

```bash
nohup <dev command> > "${TMPDIR:-/tmp}/run-<repo>-$PORT.log" 2>&1 &
```

Then poll that file for the same readiness line Step 5 describes. The `nohup` form is the
fallback, not the default: nothing surfaces its failure to the user.

## Step 5: Wait for readiness — with a pattern the command cannot satisfy

```bash
hcall pane.wait_for_output "{\"pane_id\":\"<pane_id>\",\"source\":\"recent\",
  \"match\":{\"type\":\"regex\",\"value\":\"<ready pattern>\"},\"timeout_ms\":90000}"
```

**The pane's scrollback contains the command you just sent, so a careless pattern matches
instantly and reports ready before anything started.** Measured: waiting for `READY-PROBE-[0-9]+`
matched the echoed `echo READY-PROBE-3338` command line, not its output. Therefore:

- **Never wait on the port number** if the command mentions it. `PORT=3338 npm run dev` + a wait
  for `3338` always matches immediately.
- Wait on text only the framework prints: `ready in`, `Application is running on`,
  `Nest application successfully started`, `compiled successfully`.

### The allocated port is a request, not a result — read the port back out

**A dev server may quietly choose a different port and still say it is ready.** Measured on the
first real run of this skill: `wt` allocated 3338, and Vite printed

```
Port 3338 is in use, trying another one...
  VITE v5.4.18  ready in 1133 ms
  ➜  Local:   http://localhost:3339/
```

so the app came up on **3339** while every downstream claim would have said 3338. `ui-verifier`
pointed at 3338 would then have failed in a way that reads exactly like a broken change. Vite's
`server.strictPort: true` turns that drift into an error; without it the fallback is silent by
design. So: **take the port from the startup line, not from `wt.port`,** and report the drift when
it happens rather than the number you asked for.

### Checking the port: `lsof`, never `nc` or `curl`

```bash
lsof -nP -iTCP:"$PORT" -sTCP:LISTEN        # works under the sandbox
```

**`nc -z` and `curl` report every port as closed inside the command sandbox**, because the sandbox
allows outbound connections only to an allowlisted host — and localhost is not on it. Measured
against two ports that were genuinely listening: `nc -z` said both were free, `curl` returned
`http 000`, and `lsof` got both right. A check that always answers "free" is worse than no check,
since it launders a wrong port into something that looks verified. `lsof` queries the kernel's
socket table instead of dialling, which is why it survives.

The same defect sits in `/fix` and `/ship`, which recommend `nc -z <host> <port>` to separate a
network failure from a credential failure. That advice is sound outside the sandbox and inverted
inside it: for a **remote** host there is no `lsof` equivalent, so run those with the sandbox
disabled and say that you did.

This is the same failure as `herdr agent prompt --wait --until idle` returning at once because
the agent was already idle. Any wait primitive can be satisfied by state that predates the thing
you are waiting for; make the condition impossible to meet before the event.

## Step 6: Report the URL and the evidence

State, in the reply:

```
<repo>  http://localhost:<PORT>   ready in <N>s   pane <pane_id>
<the actual startup line, quoted>
```

The URL is not decoration — `ui-verifier` is project-agnostic and takes it from the caller, so
the port read in Step 1 has to arrive there. A verifier pointed at the wrong port fails in a way
that reads exactly like a broken change.

Then use it: exercise the changed path and quote what came back. `/run` has done its job when
there is real output to paste, not when the server started.

## Red flags

| Thought | Reality |
|---|---|
| "I'll find a free port" | The port is already allocated. `git config --get wt.port`. |
| "`portEnv` is set, so the port is wired" | It is a claim about the app. hub's is inert. Read what the code reads. |
| "The `.env` has a PORT line, that's the one" | hub's `PORT=3002` matches the default and is read by nothing. |
| "It printed the port, so it's up" | That may be the command you sent, echoed. Check the socket. |
| "I'll just run it in the background here" | Then its failure surfaces to nobody. A pane, or a log file you then read. |
| "No dev command documented, I'll infer one" | A wrong launch command looks like a broken change. Say it is missing. |
| "The server started, so the change works" | Starting is not exercising. Drive the changed path and quote the output. |
| "No `dev` script, so there's no way to run it" | Scripts may be named after the app (`npm run hub`). Print the whole `scripts` block. |
| "I'll pipe it through `tail` to keep it short" | `tail` waits for EOF a watch server never sends. You get nothing. |
| "The config dir isn't in git, so it doesn't matter" | It may *be* the config. hub's is gitignored and holds the DB credentials. |
| "I'll start the server for this repo" | Did the task touch it? If not, read the frontend's env — it may already point at staging. |
| "Port's taken, so I'll reuse it" | Whose code is it running? Another worktree's server is another task's uncommitted edit. |
| "My own port is right, so I'm wired" | That is half. `wt wire` fixes where it looks for the *other* services. |
| "This worktree's `.env` came from main, so it's current" | Frozen at creation. The baseline has moved since. `wt wire`. |
| "Port's taken, I'll use another" | Then you are testing the *other* server. Report the running URL and stop. |
| "New tab for the dev server" | Split to the **right**, keeping the agent full height. A tab only past four. |
| "Split below the agent" | That costs the agent 15 rows and caps the column at two. Right, not down. |
| "The task touches 3 repos, so: tab" | Count panes in the column, not repos. One may already be closed. |
| "One repo, so one server" | hub alone runs ten apps on distinct ports. The unit is a service. |
| "`wt` gave this worktree its port" | One port. A second app in the same worktree needs one written by hand. |
| "`wt` gave it 3338, so it's on 3338" | Vite prints "Port 3338 is in use, trying another one" and drifts. Read the port back out. |
| "`nc -z` says the port is free" | Under the sandbox `nc` says that about every port. Use `lsof`. |
| "No `CLAUDE.md`, so I must stop" | Stop on an *ambiguous* recipe. One `"dev": "vite"` script is not ambiguous. |
| "This table tells me how the port works" | It names shapes, not repos. Which one applies is a project fact — read the config. |
| "UI verify is done, I'll tidy up and stop the server" | Not yours to stop. It is live infra the user may still want. Teardown belongs to `/sweep` (after the work lands) or an explicit request — leave it running and report the URL. |

Stopping a server started this way: `hcall pane.send_keys '{"pane_id":"…","keys":["C-c"]}'`
(`ctrl-c` is rejected as `invalid_key`; `C-c` and `ctrl+c` are both accepted) — but **only when
the user asks, or at `/sweep`**. Do NOT stop it on your own as end-of-task cleanup: not after the
UI verify, not while a review agent runs, not at the end of `/fix`/`/ship`. A running dev server
is live infrastructure the user may still want to look at; the sanctioned teardown point is
`/sweep`, which runs *after* the work has merged and stops dev servers itself.
