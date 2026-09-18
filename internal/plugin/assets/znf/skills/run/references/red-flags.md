<!-- Moved verbatim from run/SKILL.md § Red flags (token-diet). Read when: you are about to justify skipping a step of /run. -->

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
