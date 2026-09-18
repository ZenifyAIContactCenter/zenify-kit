# Onboarding rationale

Background for decisions in SKILL.md — why the date-stamp exists, how model routing actually
works, and why there is no Step 8.

## Why date-stamp the map

This is the cheapest defence against the failure mode that actually bites: not a wrong map, but
a map that was *right when written* and drifted. Real case in this workspace — a project's
`CLAUDE.md` correctly described a monolith with `start:dev`/`start:debug`, a teammate converted
the repo to an 8-app monorepo and those scripts disappeared, and the doc stayed authoritative
across at least three later sessions with nothing to signal it had gone stale. A stamp turns
silent drift into a visible question: *is this still true, 400 commits later?*

## Model routing rule, in full

A **skill** runs in the main loop and therefore uses whatever the session model is (`/model`);
only a **subagent** can pin its own model, via its `agents/*.md` frontmatter or the dispatch
call's `model` param, and a subagent with no `model` set runs on `CLAUDE_CODE_SUBAGENT_MODEL`
(the kit sets `sonnet` via `zenify up`), falling back to the session model only where that env is
unset. So a skill can never "escalate to Opus" on its own: for a step that must be Opus
(brainstorm, plan, review), either keep the session on Opus or route that step through a
subagent dispatched with `model: 'opus'` (the dispatch tool's param is an enum of tiers;
frontmatter accepts a full ID such as `model: claude-opus-4-8`).

## Why there is no Step 8

An earlier version of this skill had a Step 8: "inform the `project-manager` agent". That agent
is gone, and the step was hollow anyway — it wrote a status file into `~/.claude/plans/`, which
nothing reads. A project's status lives in its own spec and plan files under
`docs/superpowers/` and in the SDD ledger at `.znf/sdd/<plan>/progress.md`, both produced as a
side effect of doing the work — so there is nothing to register by hand.
