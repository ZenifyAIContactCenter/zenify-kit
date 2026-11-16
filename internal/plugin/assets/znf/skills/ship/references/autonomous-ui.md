<!-- Read when: ship step 4 or step 7 under ZNF_AUTONOMOUS=1 (worker sandbox). -->

## Step 4 under ZNF_AUTONOMOUS=1

The step 4 file-count command non-zero means the diff renders something. Branch on `ZNF_SANDBOX_WEB_PORT`:

- **Set** (the image has a web app):
  - `ZNF_SANDBOX_UI_STACK` set: run `"$ZNF_SANDBOX_UI_STACK" <web repo dir>` as the whole command, nothing piped or appended (exit 0 = ready, 3 = not). Read its last stdout line, one JSON object:
    - `{"ready":true}`: go on below.
    - `{"ready":false,"reason":"<code>","log":"<path>"}`: run `zenify ui-verify blocked --reason <code> --repo <web worktree>`, do not dispatch the verifier, and continue the gate (step 7's check then reports `infra-blocked`).
  - `ZNF_SANDBOX_UI_STACK` unset: start the web app per the autonomous sandbox notes of the `znf:run` skill.
  - Log in with `zenify e2e login --port $ZNF_SANDBOX_WEB_PORT --role <role>` (fails: run `zenify ui-verify blocked --reason drive_failed:login --repo <web worktree>`, do not dispatch) and dispatch `znf:ui-verifier` exactly as in human mode. In its brief, give the merge-base and list the `.md` files of the `<web worktree>/.znf/features/` directory (missing or empty = none). A journey runs with `zenify e2e run --local --port $ZNF_SANDBOX_WEB_PORT`.
  - Network evidence (`zenify browser requests`, `request <n>`): cite method, URL path and status on the board and in the PR; never copy headers, cookies, tokens or response bodies containing them.
  - Branch on the verifier's report: any FAIL wins, then any BLOCKED or PARTIAL; PASS only when every item is PASS.
    - PASS: it recorded the verdict; step 7 passes.
    - BLOCKED or PARTIAL (it could not drive the app: login failed, route unreachable, a feature map step or selector is gone): run `zenify ui-verify blocked --reason drive_failed:<entry name or route it names> --repo <web worktree>`. Not a UI bug, so no fix loop.
    - FAIL (it drove the app and the UI is wrong): enter the ship fix loop like any failing check, and re-dispatch the verifier after each fix. The verifier records the failing screens with `--verdict fail`. Still FAIL after round 2: run no `blocked` (it is refused anyway); the state stays `required`, so `zenify ui-verify check` writes this fix loop's `needs-human` await (exit 8): end the session.
  - `--reason` is `<code>` or `<code>:<detail>`: code `[a-z_]{1,40}`, detail reduced to `[A-Za-z0-9._-]{1,60}` (a route `/deals/:id` becomes `deals-id`, an entry name keeps only those characters). Anything else is refused (exit 2). `blocked` is also refused once a non-pass verdict is recorded for the tree: a FAIL is never infra.
- **Unset** (no web app in the image): do not dispatch the verifier. Run step 7's `zenify ui-verify check --repo … --base …`: exit 8 means the await is written, so end the session; verified or waived passes.

## Step 7 under ZNF_AUTONOMOUS=1

`zenify ui-verify check` runs in every case above; with a verdict recorded for HEAD it passes. Exit 8 stays the await stop: end the session, no BLOCKED line.

A blocked record is tied to the tree it was written for. If this run hit an infra-blocked outcome at step 4 (ui-stack not ready, or `drive_failed`) and the tree changed since (a review-loop edit), re-run ui-stack (or the driving attempt) for the current HEAD before `check`; still not ready, run `zenify ui-verify blocked --reason <code> --repo <web worktree>` again. Never after a FAIL.

State `infra-blocked: <code>` exits 0: add `⚠️ UI chưa kiểm: <code>` to the board and the exact same line to the PR body, then ship as usual (gate log `SHIPPED`).
