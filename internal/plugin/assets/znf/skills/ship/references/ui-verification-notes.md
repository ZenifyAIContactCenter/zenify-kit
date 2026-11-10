<!-- Moved verbatim from ship/SKILL.md § 4. Behavioural verification (W4 slim-skills). Read when: the UI verifier stalls, logs itself out, you are tempted to skip the look (flag OFF / needs seed / backend down), or you doubt why the check runs here and not in /cook. -->

**Contents:** the authoritative UI pass · mechanical trigger · skips that don't hold · bringing the app to a testable state · login handover · the `ui-verify record/check` gate.
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
A non-zero `zenify ui-verify files --count` means the diff renders something; a verifier verdict is then required, or the ship
is BLOCKED (board line ❌, Shippable NO). None of the following earns a skip — each is the shape of
rationalising one:

| "I skipped the look because…" | Why it does not hold |
|---|---|
| the feature is behind a flag / env-gate that is OFF in dev | The OFF surface still renders and your diff can break it — verify non-regression. Then turn the flag ON (recipe below) and verify the new state; that IS the check. |
| the enabled path needs DB config / a seeded row / a view-as user | Setup you perform, not a blocker you defer. Bring the app to the state that shows the change. |
| the change only wires an existing component to new data | It still alters what the user sees. Look at it. |
| the DB / backend was down when I got here | Then the look did **not** run: line ❌ and Shippable NO. Bring them up (recipe below), or STOP and tell the user — never green a check that never ran. |

The escape phrase "nothing renders in this diff" belongs to a literal `0` from `zenify ui-verify files --count` only —
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
   a permission/flag/tenant-setting row or a view-as user is reviewable. Dry-run it, then run it
   yourself with `zenify data run --stg --write` (`_shared/data-scripts.md`). Ground the real collection +
   user with `zenify db-read` first.
4. Only then dispatch `znf:ui-verifier` onto the already-authenticated, already-enabled screen.

If you genuinely cannot reach that state this session, the look is BLOCKED, not passed: mark the line ❌,
set Shippable to NO, and hand the user the one concrete thing that is missing. A green ship whose UI was
never looked at is the exact failure this gate exists to prevent.

### From § 4c: login handover to the verifier
Two things this avoids, both of which have actually happened: a verifier stalling for a human because
it could not authenticate, and a verifier being handed an *old* session whose screens are empty by
design, so the check verified nothing.

The mechanics: the verifier logs itself in. Before its first navigate it runs
`zenify e2e login --url <app base URL>` via Bash and runs `zenify browser -s=<session> state-load <path>` with the path
that prints — nobody types a password through a tool call, which the classifier blocks. If the command
fails or is missing, the verifier reports BLOCKED with its message. The main session only passes the
web URL and does not touch the browser to log in. Tell the verifier **not** to clear `localStorage`
or cookies — one logged itself out mid-run.

Snapshots, console dumps and screenshots are the largest volume any step here produces, and it returns
a verdict plus evidence instead.

#### Mechanical gate: ui-verify record/check
`znf:ui-verifier` records its own artifact. It loops over every screen and every width printed by `zenify ui-verify viewports --repo <repo>` (nothing printed = no declared widths, it picks one). For each (screen, viewport), if `zenify` is on PATH, it runs `zenify ui-verify record --repo <repo> --screen <name> --viewport <w> --screenshot <path> --measurement '<json>' [--fail-reason '<text>']`, where `<json>` is the whole, unedited multi-line JSON stdout of the measurement snippet. The kit computes the verdict: it requires `innerWidth == --viewport`, computes overflow per edge beyond the container content edge (> 1px, skipping an axis whose container scrolls), and fails on any overflow or a non-empty `--fail-reason`. It prints `verdict:`, `overflow:` and `failReason:` lines. It computes a fingerprint `fp` over the caller-passed repo's working tree, copies the screenshot to `<repo>/.znf/ui-verify/<fp>-<screen>-<viewport>.png`, and upserts the entry keyed by (screen, viewport) into `<repo>/.znf/ui-verify/<fp>.json`. The `--repo` the caller passes to the verifier must be this worktree.

`/ship` step 7 runs `zenify ui-verify check --repo <path> --base "$(git merge-base "$(zenify wt base)" HEAD)"` before concluding Shippable —
a deterministic, fail-closed gate with four outcomes. The render-trigger set comes from `uiVerify.renderPaths` in `.claude/worktree.json` when declared, else the built-in pattern; test files never count:
- **not_required** (exit 0) — the render-trigger set (§4b) is empty; nothing to verify.
- **waived** (exit 0) — an ADDED line (`+`) in the branch diff (working tree vs the merge-base with base, uncommitted changes included) of a file that `zenify ui-verify files` lists carries `znf:ui-verify-ok: <reason>` with a non-empty reason (a renamed file is not an addition, so its old marker does not count; nor does a marker in an untouched line, a test file or a non-render file; git errors mean no waiver); the reason prints on
  the `look:` board line.
- **verified** (exit 0) — every screen has an entry for every declared viewport at the *current* fingerprint (no declared viewports: any one viewport per screen), and every entry passes when the kit recomputes it from the stored layout and `failReason` (the stored `verdict` is ignored).
- **required** (exit non-zero) — render-trigger fired, a screen/viewport is missing or failed (e.g. `ticket-detail: missing viewport 1280`, `home@1366: fail (right 5px)`): `look:` reads
  `❌ BLOCKED`, `Shippable: NO`.

Before the waiver and artifact steps, `check` also runs the style lint (`zenify ui-verify style`) when `uiVerify.style` declares rules (`hex`, `palette`, `arbitrary`; none declared = no change). It examines only lines this branch added in `.tsx`/`.jsx`/`.vue`/`.svelte` render files, and every line of a new untracked render file counts as added. A violation prints state `style` with a message starting `ui-verify style —` and exits 1; `znf:ui-verify-ok` does not waive it, only `znf:ui-style-ok: <reason>` on that line or the line above does. Under `ZNF_AUTONOMOUS=1` it still exits 1 and opens no needs-human await: the agent fixes the line itself, since only a missing verdict needs a person.

The fingerprint is what ties `record` to `check`: any working-tree change after `record` shifts `fp`,
so a stale artifact from before a review-loop fix cannot pass `check` — the gate re-requires a fresh
look, the same honesty mechanism as step 7's `fp` stamps elsewhere in this skill.

### Bringing the app to a testable state (moved from § 4)
A flag shipping OFF, an unseeded path or a downed backend is setup you perform, never a skip:
`Skill(znf:run)` for FE and BE, `wt wire` to point the FE at this worktree's BE, turn the flag on,
seed the row — then look. If you cannot this session, that is BLOCKED, and the board names the one
missing thing.

Also moved here: require the measurement next to an *unchanged sibling*, so an overflow that was
already there is not reported as yours. A multi-screen
change is one verifier covering every screen; if you do split it, each verifier gets its own `zenify browser` session.

### Applying route proposals
Only at ship step 4, never in `/cook`'s per-task verify: a baseline must capture the state that will merge. For each `route-proposal:` line the verifier returned, run in order:

1. `zenify visual add-route --repo <path> --name … --path … [--wait-for …] [--mask …] [--aria …]`
2. `zenify visual check --repo <path> --port <P> --update`
3. `git -C <path> add -f` the files `add-route` printed (`routes.json` and the route's baselines; staged counts as tracked)
4. `zenify visual check --repo <path> --port <P>` (no update)

`--update` rewrites every baseline that differs, not only the new route's. After step 2, `git -C <path> status --porcelain -- .znf/visual/__snapshots__` must list only the new route's files; if it lists another baseline, restore it with `git -C <path> checkout -- <file>` and decline the proposal.

If any of steps 1-4 fails, undo that route: unstage with `git -C <path> reset -q -- <files>`, restore `git -C <path> checkout -- .znf/visual/routes.json` (delete the file if you just created it), delete the baseline files just generated, and record the reason. A declined proposal never turns Shippable to NO. Adding a route changes the fingerprint, so any check stamped with the old `fp` must re-run.

The optional route field `aria` is a root selector whose accessibility tree is compared against `.znf/visual/__snapshots__/<name>.aria.yml` with `deep-equal`, so an added element fails as well as a missing one. `visual check` refuses to run when `routes.json` or an on-disk baseline is untracked: `baseline not tracked by git — run: git -C <repo> add -f …`.
