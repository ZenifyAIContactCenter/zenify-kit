<!-- Moved verbatim from run/SKILL.md § Step 2, "Then wire the peers", "Then stop" (W4 slim-skills). Read when: the port you allocated is not the port the app listens on, or peers still point at main-checkout ports. -->

### From § Step 2: How the port reaches the app is per-repo, and must be read from code
An earlier version listed repos by name and asserted one of them read `PORT` via Next. It was Vite,
the default was not the one stated, and the value arrived through the env *file* rather than the
environment — three wrong claims in one row, none of which mattered until they did. A named row is
exactly the thing that stops being true, quietly, while still reading as authority.

The third one is the trap, and it is invisible from config alone: hub's `.env` contains
`PORT=3002`, which *matches the hardcoded default*, so the setup looks wired when nothing reads
it. Every hub worktree then listens on 3002 — colliding with the main checkout and with every
other hub worktree.

Adding a global env-variable mapping makes every documented-but-inert name live in *all* environments
at once, including production, where a `.env` written to match a stale README may already set one.
That is a deployment change disguised as a dev-environment fix, and it is the user's call, not this
skill's.

### From § Then wire the peers — before starting anything
Getting the app's **own** port right is only half of it. The other half is where it looks for the
*other* services, and that is where the silent pass lives: a frontend worktree whose env still
points at the baseline is testing against the **unchanged** backend, and it goes green.

A worktree's env file is copied at creation and frozen there, while the main checkout's moves on.
Measured on a real worktree three days old: it still carried `localhost:3001` / `localhost:3002`
after the baseline had been changed to point at a deployed environment — so it was aimed at
whatever happened to be occupying those ports. Re-syncing the baseline is the same command.

### From § Then stop, if something is already serving it
The third row is the one that bites, because reuse looks free and the contamination is invisible:
your change verified green against a server running someone else's half-finished edit.

Measured, on this skill's own demo run — `be` and `hub` were started locally while the frontend's
`.env` pointed both of them at `*-staging`, so both ran for nothing.
