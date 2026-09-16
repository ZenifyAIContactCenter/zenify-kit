<!-- Moved verbatim from ship/SKILL.md § 4. Behavioural verification (W4 slim-skills). Read when: the UI verifier stalls, logs itself out, you are tempted to skip the look (flag OFF / needs seed / backend down), or you doubt why the check runs here and not in /cook. -->

### From § 4a: this is the authoritative UI pass
**This is the authoritative UI pass — the last one, not the only one.** `/ship` is invoked
unconditionally by `/cook`, `/fix` and `/hotfix`, so a rendering change always reaches it without the
step being copied into three pipelines. It runs *here* because it must happen at the **final
fingerprint**: a review fix in step 5 can break layout, and a check taken before that fix certifies a
tree that no longer exists.

`/cook` step 6 also looks, once per rendering task, before writing that task's ledger line. The two do
not overlap: the per-task check tells you **which task** broke a layout, and cannot see a layout broken
three tasks later; this one catches what a *review* fix breaks, and cannot attribute it to a task.
Neither substitutes for the other, and `/fix` and `/hotfix` have no per-task equivalent, so for them
this is genuinely the only look.

### From § 4b: mechanical trigger
The agent exists for exactly this and says why in its own description: it *"keeps heavy browser output
out of the main context"*.

It renders in a pinned Docker image so the baseline is OS-independent.

#### Non-zero is non-negotiable — the skips that don't hold
A non-zero `rg` count means the diff renders something; a verifier verdict is then required, or the ship
is BLOCKED (board line ❌, Shippable NO). None of the following earns a skip — each is the shape of
rationalising one:

| "I skipped the look because…" | Why it does not hold |
|---|---|
| the feature is behind a flag / env-gate that is OFF in dev | The OFF surface still renders and your diff can break it — verify non-regression. Then turn the flag ON (recipe below) and verify the new state; that IS the check. |
| the enabled path needs DB config / a seeded row / a view-as user | Setup you perform, not a blocker you defer. Bring the app to the state that shows the change. |
| the change only wires an existing component to new data | It still alters what the user sees. Look at it. |
| the DB / backend was down when I got here | Then the look did **not** run: line ❌ and Shippable NO. Bring them up (recipe below), or STOP and tell the user — never green a check that never ran. |

The escape phrase "nothing renders in this diff" belongs to a literal `0` from the `rg` count only —
never to an argument that the rendered output is unchanged.

#### Recipe: bring the app to a testable state
A UI change is verified only once the changed surface is actually on screen. This is part of the gate,
not optional prep:

1. **Start both sides on their allocated ports.** `Skill(znf:run)` for the FE *and* the backend it
   calls, then `wt wire` so the FE points at the local BE, not staging. `znf:run` reads
   `git config --get wt.port` — it never hunts for a free port, and its URL is what the verifier needs.
2. **Flip the flag/env-gate ON in the worktree.** Write the enabling variable into the worktree's env
   file (the file `wt` seeds) and restart the FE so the bundler re-reads it. A flag left OFF shows the
   old surface and verifies nothing new.
3. **Seed the config the enabled path needs through a script the user can see.** A DB write goes in a
   purpose-written script (never `zenify db-read`, which is read-only; never a raw mutate) — so seeding
   a permission/flag/tenant-setting row or a view-as user is reviewable. Ground the real collection +
   user with `zenify db-read` first.
4. Only then dispatch `znf:ui-verifier` onto the already-authenticated, already-enabled screen.

If you genuinely cannot reach that state this session, the look is BLOCKED, not passed: mark the line ❌,
set Shippable to NO, and hand the user the one concrete thing that is missing. A green ship whose UI was
never looked at is the exact failure this gate exists to prevent.

### From § 4c: login handover to the verifier
Two things this avoids, both of which have actually happened: a verifier stalling for a human because
it could not authenticate, and a verifier being handed an *old* session whose screens are empty by
design, so the check verified nothing.

The mechanics: neither verifier gets past a login — they have the eight ordinary browser tools and
**not** `browser_run_code_unsafe`, and their attempt to read credentials is classifier-blocked. So the
main session logs in itself — `browser_snapshot` for the refs, then `browser_type` into the fields and
`browser_click` the button, values read from the workspace `settings.local.json` — and only then
dispatches the verifier onto the already-authenticated browser. If a `browser_type` carrying a password
is refused once with *"Stage 2 classifier error — usually transient, retrying often succeeds"*, that
means what it says: retry, do not start building a way around it. Tell the verifier **not** to clear
`localStorage` or cookies — one logged itself out mid-run.

Snapshots, console dumps and screenshots are the largest volume any step here produces, and it returns
a verdict plus evidence instead.

#### Mechanical gate: ui-verify record/check
`znf:ui-verifier` records its own artifact — before deleting its screenshot, if `zenify` is on PATH it
runs `zenify ui-verify record --repo <repo> --screen <name> --screenshot <path> --child-right <n>
--container-right <n> --padding-right <n> --verdict <pass|fail>`. This computes a fingerprint `fp` over
the caller-passed repo's working tree, copies the screenshot to `<repo>/.znf/ui-verify/<fp>-<screen>.png`,
and upserts the screen's measurement into `<repo>/.znf/ui-verify/<fp>.json`. The `--repo` the caller
passes to the verifier must be this worktree.

`/ship` step 7 runs `zenify ui-verify check --repo <path> --base <base>` before concluding Shippable —
a deterministic, fail-closed gate with four outcomes:
- **not_required** (exit 0) — the render-trigger set (§4b) is empty; nothing to verify.
- **waived** (exit 0) — the diff carries a `// znf:ui-verify-ok: <reason>` marker; the reason prints on
  the `look:` board line.
- **verified** (exit 0) — a valid artifact exists for the *current* fingerprint.
- **required** (exit non-zero) — render-trigger fired, no valid current-fp artifact: `look:` reads
  `❌ BLOCKED`, `Shippable: NO`.

The fingerprint is what ties `record` to `check`: any working-tree change after `record` shifts `fp`,
so a stale artifact from before a review-loop fix cannot pass `check` — the gate re-requires a fresh
look, the same honesty mechanism as step 7's `fp` stamps elsewhere in this skill.
