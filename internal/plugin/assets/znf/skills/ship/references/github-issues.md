# GitHub issues — link one before the PR

Read when: the session has a line starting `features: github.issues on`. No such line → skip this
whole file; ship opens the PR as before.

1. **Find the issue, in this order:** a number the user gave in this session (including an
   `Issue: #N` line in a nonstop brief) → an issue named in the spec or the SDD ledger → create one.

2. **Before creating, look for overlap.** Run `gh pr list --state open` and `git worktree list`,
   then `gh issue list --state open --search "<words>"` and `gh pr list --state open --search "<words>"`,
   where `<words>` is the branch slug after `<type>/` with each `-` replaced by a space. Any hit →
   STOP and ask the user, giving the full URL of each hit.

3. **Create:**
   `gh issue create --assignee @me --title "<PR title description>" --body "<Brief Problem>"` —
   the title is the PR title's description (after the colon), the body is the spec Brief's Problem
   in the project's prose language. No label.

4. **PR body:** a `Closes #N` line (never `Refs #N`). Several repos → each PR closes the issue in
   its own repo.

5. **The user says no issue is needed** → skip the issue and state that reason in the PR body.

6. **Base is not the default branch** (`gh repo view --json defaultBranchRef`) → say in the report
   that the issue closes by itself only when the commit reaches the default branch.

7. **Stacked ship:** one issue for the whole stack; `Closes #N` goes in the top layer's PR body
   only.

8. **Report** every issue and PR by its full URL.
