# GitHub issues — link one before the PR

Read when: the session has a line starting `features: github.issues on`, or the task brief has a
line `Features: github.issues on` (a nonstop worker never sees SessionStart; its brief carries the
orchestrator's line). Neither → skip this whole file; ship opens the PR as before.

1. **Find the issue, in this order:** a number the user gave in this session (including an
   `Issue: <owner>/<repo>#N` line in a nonstop brief) → an issue named in the spec or the SDD
   ledger → create one.

2. **Before creating, look for overlap.** Run `gh issue list --state open --search "<words>"` and
   `gh pr list --state open --search "<words>"`, where `<words>` is the branch slug after `<type>/`
   with each `-` replaced by a space. Only these two queries decide: any hit → STOP and ask the
   user, giving the full URL of each hit.

3. **Create:** write the body to a temp file in the session scratchpad first, then
   `gh issue create --assignee @me --title '<PR title description>' --body-file <file>` — the
   title is the PR title's description (after the colon), the body is the spec Brief's Problem in
   the project's prose language. No label. Never put free text (issue or PR title, body, comment)
   inside a double-quoted shell argument: backticks and `$(...)` there run as commands. Body text
   always goes through `--body-file`; a title with a `'` goes through a quoted heredoc variable.

4. **PR body:** the issue lives in one repo, `<owner>/<repo>#N`. The PR in that same repo writes
   `Closes #N`; a PR in any other repo writes `Closes <owner>/<repo>#N` — a bare `#N` there
   would close that repo's own issue N. Never `Refs #N`. Report and record the issue as the full
   `<owner>/<repo>#N`.

5. **The user says no issue is needed** → skip the issue and state that reason in the PR body.

6. **Base is not the default branch** (`gh repo view --json defaultBranchRef`) → say in the report
   that the issue closes by itself only when the commit reaches the default branch.

7. **Stacked ship:** one issue for the whole stack; `Closes #N` goes in the top layer's PR body
   only.

8. **Report** every issue and PR by its full URL.
