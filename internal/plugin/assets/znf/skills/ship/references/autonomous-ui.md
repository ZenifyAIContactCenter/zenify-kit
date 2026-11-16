<!-- Read when: ship step 4 or step 7 under ZNF_AUTONOMOUS=1 (worker sandbox). -->

## Step 4 under ZNF_AUTONOMOUS=1

The step 4 file-count command non-zero means the diff renders something. Branch on `ZNF_SANDBOX_WEB_PORT`:

- **Set** (the image has a web app):
  - `ZNF_SANDBOX_UI_STACK` set: run `"$ZNF_SANDBOX_UI_STACK" <web repo dir>` as the whole command, nothing piped or appended (exit 0 = ready, 3 = not). Read its last stdout line, one JSON object:
    - `{"ready":true}`: go on below.
    - `{"ready":false,"reason":"<code>","log":"<path>"}`: run `zenify ui-verify blocked --reason <code> --repo <web worktree>`, do not dispatch the verifier, and go to step 7.
  - `ZNF_SANDBOX_UI_STACK` unset: start the web app per the autonomous sandbox notes of the `znf:run` skill.
  - Log in with `zenify e2e login --port $ZNF_SANDBOX_WEB_PORT --role <role>` and dispatch `znf:ui-verifier` exactly as in human mode. In its brief, give the merge-base and list the repo's feature map files (`ls <web worktree>/.znf/features/*.md`; none is fine). A journey runs with `zenify e2e run --local --port $ZNF_SANDBOX_WEB_PORT`.
  - Branch on the verifier's report:
    - PASS: it recorded the verdict; step 7 passes.
    - BLOCKED (it could not drive the app: login failed, route unreachable, a feature map step or selector is gone): run `zenify ui-verify blocked --reason drive_failed:<entry name or route it names> --repo <web worktree>`. Not a UI bug, so no fix loop.
    - FAIL (it drove the app and the UI is wrong): enter the ship fix loop like any failing check, and re-dispatch the verifier after each fix. Still FAIL after round 2: record nothing and run no `blocked`; the state stays `required`, so `zenify ui-verify check` writes this fix loop's `needs-human` await (exit 8): end the session.
- **Unset** (no web app in the image): do not dispatch the verifier. Run step 7's `zenify ui-verify check --repo … --base …`: exit 8 means the await is written, so end the session; verified or waived passes.

## Step 7 under ZNF_AUTONOMOUS=1

`zenify ui-verify check` runs in every case above; with a verdict recorded for HEAD it passes. Exit 8 stays the await stop: end the session, no BLOCKED line.

State `infra-blocked: <code>` exits 0: add `⚠️ UI chưa kiểm: <code>` to the board and the exact same line to the PR body, then ship as usual (gate log `SHIPPED`).
