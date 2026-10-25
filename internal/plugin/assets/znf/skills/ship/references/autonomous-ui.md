<!-- Read when: ship step 4 or step 7 under ZNF_AUTONOMOUS=1 (worker sandbox). -->

## Step 4 under ZNF_AUTONOMOUS=1

The step 4 file-count command non-zero means the diff renders something. Branch on `ZNF_SANDBOX_WEB_PORT`:

- **Set** (the image has `contact-center-web`): start the web app per `skills/run/references/autonomous-sandbox.md`, log in with `zenify e2e login --port $ZNF_SANDBOX_WEB_PORT --role <role>`, and dispatch `znf:ui-verifier` exactly as in human mode. A journey runs with `zenify e2e run --local --port $ZNF_SANDBOX_WEB_PORT`.
- **Unset** (no web app in the image): do not dispatch the verifier. Run step 7's `zenify ui-verify check --repo … --base …`: exit 8 means the await is written, so end the session; verified or waived passes.

Step 7's `ui-verify check` runs in both cases; with a verdict recorded for HEAD it passes, and exit 8 stays the await stop (no BLOCKED line).
