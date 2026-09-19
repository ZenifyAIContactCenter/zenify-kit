---
name: run
description: Launch the app and produce real output from the real code path, so a change can be verified rather than asserted. Use when a change is behavioural and there are no tests covering it, before claiming it works, and before dispatching znf:ui-verifier (which needs the URL this produces). Reads the port the worktree was allocated instead of hunting for a free one.
allowed-tools: Read Grep Glob Bash(git *) Bash(rg *) Bash(cat *) Bash(nc *) Bash(curl *) Bash(node *) Bash(tail *) Bash(grep *)
---

`CLAUDE.md` rule #3: *"When there are no tests, produce output from the real code path and show
it."* This skill is how: it makes output exist, it does not judge it. Why a skill and not a bash
line: `references/why-this-skill-exists.md`.

## Step 1: The port is already decided — read it, never hunt for it

`wt` allocated one port per worktree and recorded it. Hunting throws that away.

```bash
PORT=$(git config --get wt.port)      # inside a wt worktree
```

Empty means this is not a `wt` worktree. Say which, and use the project's documented default;
do not invent one.

### Then stop, if something is already serving it

```bash
lsof -nP -iTCP:"$PORT" -sTCP:LISTEN            # never `nc` — see Step 5
```

Something there → do **not** start a second: it drifts to the next port, so every URL you report is
wrong. **Reuse depends on whose code it runs.** Ask the process, not the port:

```bash
PID=$(lsof -nP -iTCP:"$PORT" -sTCP:LISTEN -t | head -1)
lsof -a -p "$PID" -d cwd -Fn | grep '^n' | sed 's/^n//'      # `-a`, or lsof ORs the selectors
```

| That cwd is | Then |
|---|---|
| the repo's **main checkout** | reuse it — unmodified baseline |
| **your own worktree** | reuse it — it is your code |
| **another worktree** | **do not reuse** — another task's uncommitted code. Say whose it is and stop |

**A task only needs a local server for a repo it actually touched.** Read the frontend's env
first: if it points at staging, a local backend serves nobody (same reference).

## Step 2: How the port reaches the app is per-repo — read it from code

**`portEnv` in `.claude/worktree.json` is a claim about the app, and it can be wrong.** `wt` writes
`<portEnv>=<port>` into the env file whether anything reads it or not. Three shapes:

| Shape | Recognise it by | What to do |
|---|---|---|
| code reads the env var | `process.env.<NAME>` at the listen site | nothing; `wt` wrote it into the env file |
| config reads the **env file** | a bundler config calling `loadEnv(...)`, not `process.env` | nothing — it works because `wt` wrote the *file*, not the variable |
| **no env path at all** | a config library reading `*.yml`/`*.json`, no `${}`, no env mapping | `portEnv` **cannot work**; write a gitignored per-worktree override |

**Which shape a repo is, is a project fact — read it in that repo's `CLAUDE.md`, never carry it
between projects.**

**For the third shape, write a gitignored per-worktree override — never reach for an env var.**
node-config loads `local.EXT` after `default.EXT` (`node_modules/config/lib/config.js:457`), and
nothing creates that file for you: **this is `/run`'s job on every launch**.

```bash
# write it when absent, or when it disagrees with the port wt allocated
printf 'service:\n  hub:\n    port: %s\n' "$(git config --get wt.port)" > config/local.yml
NODE_ENV=development node -e 'console.log(require("config").get("service.hub.port"))'
```

Check `git status --porcelain` — if the override shows as a change it is **not** gitignored here
and would dirty the branch; stop and say so. Confirm at the config layer *and* at the socket.
**Never teach the config library to read env variables** — that is a deployment change disguised as
a dev fix (`references/port-wiring-rationale.md`).

Before running an app whose port shape you have not read this session:

```bash
rg -n "listen\(|env\.PORT|process\.env\.[A-Z_]*PORT" -g '!node_modules' | head
```

Find what the code reads, then make the allocated port reach *that*.

### Then wire the peers — before starting

```bash
wt wire            # --dry-run first if you want to see it
```

Your own port is half of it; the other half is where the app looks for the *other* services, and a
frozen worktree env silently tests against the unchanged backend. **Run it even for a one-repo
task, and before the server starts** — a bundler reads env files at config time, so a later wire
needs a restart (`references/port-wiring-rationale.md`).

## Step 3: The launch command comes from the project, not from memory

Read the `Commands` section of the repo's `CLAUDE.md`. **Stop when the recipe is ambiguous, not
merely when `CLAUDE.md` is absent** — several plausible candidates, or none. A wrong launch command
looks like a broken change. **"No `dev` script" does not mean no dev command**: scripts are often
named after the **app**, not the mode. Read the whole `scripts` block:

```bash
node -e 'console.log(Object.keys(require("./package.json").scripts).join("\n"))'
```

## Step 4: Run it detached, not in this session

Run it detached into a log file naming the repo and the port, then read it:

```bash
nohup <dev command> > "${TMPDIR:-/tmp}/run-<repo>-$PORT.log" 2>&1 &
```

Say the log path in the report. **Send the bare command — never pipe a watch server through `tail`
or `head`.** Reuse an existing server for this repo's port first. The unit is a **service**, not a
repo, and `wt` allocates one port per worktree — `references/launch-and-readiness.md`.

## Step 5: Wait for readiness — on a pattern the command cannot satisfy

```bash
LOG="${TMPDIR:-/tmp}/run-<repo>-$PORT.log"
for i in $(seq 1 90); do grep -qE '<ready pattern>' "$LOG" && break; sleep 1; done
grep -qE '<ready pattern>' "$LOG" || { echo "not ready after 90s"; tail -n 40 "$LOG"; }
```

A careless pattern matches the echoed command and reports ready before anything ran:

- **Never wait on the port number** if the command mentions it: `PORT=3338 npm run dev` + a wait
  for `3338` matches immediately.
- Wait on text only the framework prints: `ready in`, `Application is running on`,
  `Nest application successfully started`, `compiled successfully`.

### The allocated port is a request, not a result — read it back out

A dev server may choose a different port and still say it is ready (Vite prints `Port 3338 is in
use, trying another one...`). **Take the port from the startup line, not from `wt.port`,** and
report the drift.

### Checking the port: `lsof`, never `nc` or `curl`

```bash
lsof -nP -iTCP:"$PORT" -sTCP:LISTEN        # works under the sandbox
```

**`nc -z` and `curl` report every port as closed inside the command sandbox** — only allowlisted
hosts are dialable, and localhost is not one. For a **remote** host there is no `lsof` equivalent:
run that check with the sandbox disabled and say so (`references/sandbox-port-checks.md`).

## Step 6: Report the URL and the evidence

State in the reply:

```
<repo>  http://localhost:<PORT>   ready in <N>s   pane <pane_id>
<the actual startup line, quoted>
```

The URL is not decoration — `znf:ui-verifier` takes it from the caller, so the Step 1 port must
arrive there. Then exercise the changed path and quote what came back: `/run` is done when there is
real output to paste, not when the server started.

Stop a server only when the user asks, or at `/sweep` — never as end-of-task cleanup:
`kill` the pid from `lsof -nP -iTCP:$PORT -sTCP:LISTEN -t`.

## References

Materialized at `~/.claude/skills/znf/skills/run/references/`. Read one when its trigger fires.

- `references/why-this-skill-exists.md` — read when wondering why `/run` is a skill, not a bash line.
- `references/port-wiring-rationale.md` — read when the allocated port is not the one the app listens on, or peers still point at main-checkout ports.
- `references/sandbox-port-checks.md` — read when `nc`/`curl` says a port is closed.
- `references/launch-and-readiness.md` — read when the server will not start, readiness misbehaves, or a monorepo needs several servers.
- `references/red-flags.md` — read before justifying skipping a step of `/run`.
