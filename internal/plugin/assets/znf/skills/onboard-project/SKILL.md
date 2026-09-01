---
name: onboard-project
description: Bootstrap or map a project so the agent has an accurate system map before doing feature work. Use at the start of working in a project — when there is no CLAUDE.md yet, when joining an unfamiliar codebase, or when starting a brand-new project from scratch. The global harness (git-guard, Stop-hook memory, /ship evaluator, /run, superpowers) is already active for every project — onboarding wires the project-specific config (deploy branches, DB access, app recipe).
disable-model-invocation: true
argument-hint: "[optional: what the new project should be]"
allowed-tools: Read Grep Glob Bash(ls *) Bash(cat *) Bash(find *) Bash(git *)
---

## Current directory
First, inspect the directory: run `ls -la`, read `package.json` if present, and check `git remote -v`. Use this to detect the stack and pick the mode.

## Mode detection

From the inspection above:
- **Directory has real source code** → run **MAP mode**.
- **Directory is empty or only has scaffolding/.git** → run **BOOTSTRAP mode**.

State which mode you picked and why, then proceed.

---

## MAP mode (existing project — reverse-engineer)

Goal: wire project-specific config + produce an accurate `CLAUDE.md` system map.

1. **Detect the stack**: language(s), framework(s), package manager, monorepo vs polyrepo, build/test/lint scripts.
2. **Map the architecture**: top-level structure, entry points, and the **shared resources / inter-service contracts** where bugs hide — databases & shared collections, message queues, pub/sub channels, HTTP APIs between services, env vars. For a read-heavy sweep, consider delegating to an Explore subagent.
3. **Assess the safety net**: are there tests? a CI gate? lint/typecheck? State what does and doesn't protect against bugs.
4. **Auto-generate project config** (draft, then confirm with user):
   - `<proj>/.claude/deploy-branches` — detect from CI config (`.github/workflows`, `Dockerfile`, `docker-compose.yml`) and git remote branch names. List candidates, ask user to confirm.
   - `<proj>/.claude/settings.json` — register lint/typecheck hooks per-stack (e.g. eslint for JS/TS, tsc for TypeScript).
   - Record how to launch the app (command, port, login) in `CLAUDE.md` under Commands, so `/run` has a recipe to follow.
5. **Write `CLAUDE.md`** covering: repo roles, stack, shared infra, inter-service contracts, and what project-specific verification command is needed. Keep it a map, not a tutorial. (~70-90 lines max — detailed per-repo info goes in each repo's own CLAUDE.md, loaded on-demand.)

**Every command you write into `CLAUDE.md` must be verified before you write it** — run it, or
at minimum confirm it exists in `package.json` scripts / `Makefile` / the equivalent. Do not
copy a command out of a README, out of a sibling project, or out of an `understand-codebase`
summary and record it as fact. A wrong launch command here is worse than no command: `/run`
reads this recipe, so recording one that doesn't exist silently disables behavioural
verification for the whole project. If you genuinely cannot verify one, write it with an
explicit `(unverified)` marker rather than stating it plainly.

The same applies to paths, aliases, and structural claims. Anything you inferred rather than
observed goes in marked as inferred — a `CLAUDE.md` auto-loads every session, so a wrong line
here is read hundreds of times and believed every time.

**Date-stamp the map.** End `CLAUDE.md` with a line like
`Map verified 2026-07-27 against commit <short-sha>` (`git rev-parse --short HEAD`). This is
the cheapest defence against the failure mode that actually bites: not a wrong map, but a map
that was *right when written* and drifted. Real case in this workspace — a project's
`CLAUDE.md` correctly described a monolith with `start:dev`/`start:debug`, a teammate converted
the repo to an 8-app monorepo and those scripts disappeared, and the doc stayed authoritative
across at least three later sessions with nothing to signal it had gone stale. A stamp turns
silent drift into a visible question: *is this still true, 400 commits later?*

**Confirm with user before finalising** (these are the 5 things the agent CANNOT auto-decide):
   1. ✋ Which branches are deploy branches (show draft → user confirms).
   2. ✋ DB read-only connection string (secret — never auto-read; user provides).
   3. ✋ Public or internal product (determines /ship-frontend scope).
   4. ✋ Login / app URL needed to run the app (`/run`), and whether a VPN is required at all — do not assume one is.
   5. ✋ Any custom conventions not visible in code.

---

## BOOTSTRAP mode (greenfield — forward-engineer)

ultrathink — foundation decisions here are hard to reverse and you carry them for the life of the project. The dominant risk is **over-engineering the foundation**, so house rule #2 (smallest thing that works) applies hardest.

1. **Scope (brainstorm first, no code)**: what is being built, for whom, the smallest useful version, and explicitly what is *out* of scope. If the user invoked superpowers brainstorming, defer to it.
2. **Choose stack & architecture — the user decides, you advise**: present 2-3 viable options with honest trade-offs (not the most elaborate one). Confirm before scaffolding. These choices are sticky.
3. **Minimal scaffold**: the smallest thing that runs (one page/endpoint that boots). Do **not** pre-create directories, abstractions, or config "for later".
4. **Safety net from day 1**: set up lint + typecheck, one example test as a harness, a basic CI gate that runs them on PR, and write the app's launch command + port into `CLAUDE.md` so `/run` can drive it.
5. **Wire project config**: create `.claude/deploy-branches` (ask user which branches deploy), create `.claude/settings.json` with lint hook for chosen stack.
6. **Write `CLAUDE.md`** from the conventions just chosen (structure, naming, patterns) so consistency holds as the code grows.

Confirm each major decision with the user; don't run ahead.

---

## What the global harness provides automatically (no setup needed)
- **git-guard** hook: blocks commit/push/merge to deploy branches + scans staged diff for secrets.
- **/ship + independent reviewer**: lint/build/contract + `code-reviewer` agent (or `review-changes` workflow for large diffs).
- **/run**: launches the app and drives it to confirm a change actually works (reads the launch recipe from the project's `CLAUDE.md`).
- **superpowers**: brainstorming, systematic-debugging, TDD, verification-before-completion.
- **`code-reviewer`, `scout`, `ui-verifier`** agents (global, model pinned per agent — `code-reviewer` at `claude-opus-4-8` by full ID, though `/ship` scales it down for a small diff; the other two sonnet). Three, not more: `db-schema-fetcher` and `project-manager` were deleted after 0 dispatches across 1427 session transcripts.
- **Model routing rule** — a **skill** runs in the main loop and therefore uses whatever the session model is (`/model`); only a **subagent** can pin its own model, via its `agents/*.md` frontmatter or the Agent tool's `model` param, and a subagent with no `model` set **inherits the session model**. So a skill can never "escalate to Opus" on its own: for a step that must be Opus (brainstorm, plan, review), either keep the session on Opus or route that step through an agent whose frontmatter names the **full model ID** (e.g. `model: claude-opus-4-8`). Do not use the bare `opus` alias anywhere: it resolves to the newest opus and silently overrides the version pinned as `"model"` in `~/.claude/settings.json`. Agent frontmatter accepts a full ID; the Agent tool's `model` param does not (it is an enum), so to reach the pinned top tier from a skill you **omit** the param rather than naming it.
- **`/cook`, `/fix`** (1-verb pipeline), **`/ground`** (anti-fabrication), **`/coding-level`** (output calibration).
- **`/review-changes`, `/contract-sweep`, `/understand-codebase`, `/multi-project-status`** workflows (opt-in, token-heavy).
- **Playwright MCP** (global): browser-driven UI testing; wire app URL + login in step 4 above.
- **Agent teams** (experimental, opt-in): set `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` in global settings.

## Wiring the kit for this project (additional steps)

After the standard MAP/BOOTSTRAP steps above, wire the global kit:

6. **Global rules symlink** (for polyrepo contract drift): if this project shares contracts with other repos, symlink the relevant global rules:
   ```bash
   mkdir -p .claude/rules
   ln -s ~/.claude/rules/contract-drift-polyrepo.md .claude/rules/contract-drift.md
   ```

7. **DB read access config**: if this project has a DB, give it a documented read-only accessor and record the command in CLAUDE.md — a wrapper that takes credentials from a designated store — not a raw connection string and not `grep <VAR> .env`. `/ground` reads this recipe, so a project without one has no grounded DB access at all. Never record the credential itself.

Step 8 used to say "inform the `project-manager` agent". That agent is gone, and the step was hollow
anyway: it wrote a status file into `~/.claude/plans/`, which nothing reads. A project's status lives in
its own spec and plan files under `docs/superpowers/` and in the SDD ledger at
`.znf/sdd/<plan>/progress.md`, both produced as a side effect of doing the work — so there is
nothing to register by hand.
