# Project engine (Step 3, `ENGINE=project`)

`zenify reviewer` prints `project` only when `ZNF_REVIEWER=project` and a CLAUDE.md declares a
`Pre-PR review:` line. The project's reviewer reads a pushed branch head before any PR exists. `BASE`
and `PACK` are Step 1's; `DECLINES` is ship's list for this round (empty standalone and on round 1).

1. **Declaration.** Walk up from the worktree to `/` reading each `CLAUDE.md`, then
   `~/.claude/CLAUDE.md`; the first line starting `Pre-PR review:` wins. It names a send tool, a wait
   tool, their parameters and the result fields. Follow it exactly; never assume a tool name, a
   parameter or a field of your own.
2. **Inputs.** Keep `BASE` (the merge-base) untouched; the send tool's base is a branch name:

   ```bash
   REPO=$(git remote get-url origin | sed -E 's#\.git$##; s#.*[:/]([^/]+/[^/]+)$#\1#')   # owner/name; pass the form the tool asks for
   BRANCH=$(git branch --show-current)
   BASE_BRANCH=$(zenify wt base | sed 's#^origin/##')
   ```

3. **Push.** Uncommitted or untracked changes → fall back (this engine reviews pushed commits only;
   never commit for it). Else `git push -u origin HEAD` (the feature branch, never a deploy branch)
   and `HEAD_SHA=$(git rev-parse HEAD)`.
4. **Send.** Call the send tool with `REPO`, `BRANCH`, `HEAD_SHA`, `BASE_BRANCH` and `declines` =
   `DECLINES` (`[{id, reason}]`, ids only from the last finished round's `open[]`). A result with
   `queued: false` and a reason is not an error: the head already has a run → wait with the `after`
   it returned; the branch has a newer head → fall back.
5. **Wait.** Call the wait tool with `REPO`, `BRANCH`, `HEAD_SHA` and that `after`, its timeout
   parameter at 55 seconds or less per call: Claude Code cuts an HTTP MCP request at 60s unless the
   server's registration sets a longer `timeout` (`claude mcp get <server>` prints `Timeout:`). Not
   done → call again, for 30 minutes in all.
6. **Map.** `check` is the verdict; each `open[]` entry becomes a finding per
   `_shared/finding-schema.md`: `dimension` `bugs`, `line` from `lines`, `title`/`file`/`issue`/`fix`
   as given, `id` kept (ship builds `declines` from it), `evidence` empty. `blocks: true` keeps its
   severity (CRITICAL/HIGH); a CRITICAL/HIGH with `blocks: false` becomes MEDIUM. `FINDINGS_JSON` =
   that array. Skip Step 4 (it drops `id`, and there is no `evidence` to check); board lines
   `review … project: <n> finding, check <check>` and `verify: project engine`; the tier line reads
   `T3: one project pass`. A failing `check` with no blocking finding (the reviewer did not finish) →
   fall back.
7. **Fall back.** Tool missing from this session, a call errors, no verdict after 30 minutes, or a
   case above → board line `review … project→agent: <reason>`, then T1/T2/T3 as if `ENGINE=agent`.
   Never fail silently.
