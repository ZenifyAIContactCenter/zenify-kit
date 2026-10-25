<!-- Read when: ZNF_AUTONOMOUS=1 (worker sandbox). Facts come from zenie `deploy/worker/AGENTS.md`. -->

## Autonomous sandbox (ZNF_AUTONOMOUS=1)

- Start only the apps the change needs: be for a be change; hub plus auth for a hub change.
- Before starting, run `node /opt/sandbox/worker-env.mjs <repo-dir>` for each repo, contact-center-be first, then contact-center-hub.
- be runs on Node 16: start it with `/opt/node16/bin/node server.js`, install with `PATH=/opt/node16/bin:$PATH npm ci`. hub uses the image's default `node`.
- Ports (per `deploy/worker/AGENTS.md`): be 3001, hub 3002, hub auth 3005. Read them there, do not hunt for free ones.
- The web app is `contact-center-web`, started with the repo's dev command (read `package.json`) on the port held in `ZNF_SANDBOX_WEB_PORT`. Read the variable, never hardcode the number (zenie `worker-env.mjs` writes `PORT=4100` into the web `.env.local` today). Run `node /opt/sandbox/worker-env.mjs <web-dir>` first; it points the FE at be 3001, hub 3002 and hub auth 3005.
- `ZNF_SANDBOX_WEB_PORT` unset means the image has no web app: no URL for `znf:ui-verifier`, so verify with real requests (login recipe in `deploy/worker/AGENTS.md`) and quote the output.
- Log in to the web app with `zenify e2e login --port $ZNF_SANDBOX_WEB_PORT --role <role>` (one of the broker E2E roles), then load the state it prints with `zenify browser -s=<session> state-load <path>` before navigating.
- Run a journey with `zenify e2e run --local --port $ZNF_SANDBOX_WEB_PORT`; `--prepare` (needs `--local`) only prepares the harness (npm and chromium) and then exits without running any test, so run `zenify e2e run --local --port $ZNF_SANDBOX_WEB_PORT` a second time to run the journeys.
