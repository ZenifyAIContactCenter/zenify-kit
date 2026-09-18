<!-- Moved verbatim from run/SKILL.md §§ Step 4, Step 5 (token-diet). Read when: the server will not start, the readiness wait misbehaves, or a monorepo needs more than one server. -->

### From § Step 4: Run it detached, not in this session
A dev server is long-lived. Started from the session's Bash it either blocks the turn or is
orphaned, and its output lands nowhere anyone can look at again.

**Reuse before creating.** A server already serving this repo's port (Step 1) is the answer to
"where does it go"; a second one for the same repo is how you end up with two servers and one of
them on a drifted port.

**The unit is a service, not a repo.** A monorepo runs several apps from one checkout, each on its
own port, so one repo can need three servers by itself and "how many repos does the task touch"
answers the wrong question. Count services you are actually starting.

**`wt` allocates one port per worktree — a real gap, not a convention.** A worktree running a
second app has no allocated port for it; that port has to be written by hand into the same
gitignored override the third shape uses, chosen from the repo's declared `portRange`. Read
the repo's `CLAUDE.md` for which apps it runs and which key each takes its port from; do not assume
the app you know is the only one.

**Never pipe a watch server through `tail` or `head`.** `tail` waits for
EOF, which a `--watch` process never reaches, so `npm run hub 2>&1 | tail -40` produces **no output
at all** — and then the readiness wait times out and the app looks broken while it is running fine.
Measured, on the first attempt at exactly this. If output volume is the worry, bound it by reading
fewer lines back (`tail -n N` on the log file), not by filtering at the source.

`--debug` in a Node watch command also means the inspector binds its default 9229, which **is not
per-worktree** — a second watch server in another worktree collides there even when the HTTP port
is correct.

If your terminal has a pane/workspace manager, a personal skill may wrap this step to give the
server its own pane. That is ergonomics on top of this recipe, never a replacement for the log file.

### From § Step 5: Wait for readiness
Whatever you wait on may already contain the command you just ran (a shell echo, a pane's
scrollback), so a careless pattern matches instantly and reports ready before anything started.
Measured: waiting for `READY-PROBE-[0-9]+` matched the echoed `echo READY-PROBE-3338` command line,
not its output.

### From § The allocated port is a request, not a result
**A dev server may quietly choose a different port and still say it is ready.** Measured on the
first real run of this skill: `wt` allocated 3338, and Vite printed

```
Port 3338 is in use, trying another one...
  VITE v5.4.18  ready in 1133 ms
  ➜  Local:   http://localhost:3339/
```

so the app came up on **3339** while every downstream claim would have said 3338. `znf:ui-verifier`
pointed at 3338 would then have failed in a way that reads exactly like a broken change. Vite's
`server.strictPort: true` turns that drift into an error; without it the fallback is silent by
design.

### From § Checking the port
The same defect sits in `/fix` and `/ship`, which recommend `nc -z <host> <port>` to separate a
network failure from a credential failure. That advice is sound outside the sandbox and inverted
inside it: for a **remote** host there is no `lsof` equivalent, so run those with the sandbox
disabled and say that you did.

### From § Step 2: config directories
**A repo whose config directory is gitignored cannot run from a bare checkout.** Seed it the same
way `.env` is seeded — `wt`'s `copy` list takes directories (`cp -c -R`, `wt:410`). Without it the
worktree silently falls back to code defaults, which for hub means `mongodb://localhost:27017/test`
and a placeholder JWT secret: a failure that reads exactly like a broken change.

**Confirm at the config layer *and* at the socket.** The config layer is where this class of bug
lives; the socket is where the claim lands. Verified once end-to-end: the override resolved to the
allocated port, the worktree's server took it, and the main checkout's kept its own — two instances
at once, impossible before.

`wt wire` recomputes each declared peer variable from scratch — the peer's worktree port when a
worktree of **this slug** exists, otherwise the value the main checkout has. So it is idempotent,
it reverts to baseline when the peer is torn down, and a *different* task's worktree is never wired
in. Declared per consumer repo, keyed by env var rather than by repo, because one repo can serve
several services:

```json
"peers": { "VITE_HUB_URL": { "repo": "…", "url": "http://localhost:{port}" } }
```
