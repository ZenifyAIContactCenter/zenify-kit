<!-- Read when: ZNF_AUTONOMOUS=1 (worker sandbox). Facts come from zenie `deploy/worker/AGENTS.md`. -->

## Autonomous sandbox (ZNF_AUTONOMOUS=1)

- Start only the apps the change needs: be for a be change; hub plus auth for a hub change.
- Before starting, run `node /opt/sandbox/worker-env.mjs <repo-dir>` for each repo, contact-center-be first, then contact-center-hub.
- be runs on Node 16: start it with `/opt/node16/bin/node server.js`, install with `PATH=/opt/node16/bin:$PATH npm ci`. hub uses the image's default `node`.
- Ports (per `deploy/worker/AGENTS.md`): be 3001, hub 3002, hub auth 3005. Read them there, do not hunt for free ones.
- There is no FE in the sandbox, so there is no URL for `znf:ui-verifier`: verify with real requests (login recipe in `deploy/worker/AGENTS.md`) and quote the output.
