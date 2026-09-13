<!-- Moved verbatim from cook/SKILL.md § Step 0: Sync the base (W4 slim-skills). Read when: the
checkout is on someone's branch and you are unsure what to read, or you want to know why Step 0
moved out of the worktree-creation step. -->

### From § Step 0: Sync the base — fetch, and know which ref you are reading

**No worktree here.** It used to be Step 0, which was wrong: Steps 1-5 only read code and write
the spec and plan, and those go in the **main checkout** by rule #8 — so nothing needs a worktree
until Step 6. `/fix` already had this right (*"create the worktree when you are about to edit, not
when you start looking"*); `/cook` was the one out of step. What Step 0 is really for is making
sure the code you are about to ground and design against is current.

`fetch` updates that ref whatever the checkout happens to have out, which matters because **main
checkouts are mostly not sitting on their base**: measured once across a 13-repo workspace, 8 were
on someone's feature or hotfix branch and 3 were dirty. A `pull` there either fails or advances the
wrong branch, and a `checkout` would abandon work in flight.

The base is *declared*, not guessed — `baseRef` in each repo's `.claude/worktree.json`, and it
differs between repos in the same workspace, so read it per repo and never carry one repo's answer
to another.

### From § Step 0 — consequence for Step 1 (extra, tier two)

Reading a file out of the main checkout gives you whatever branch that checkout is on. Ground a
repo while its checkout sits on an unrelated feature branch and you have grounded against that
branch, not the base — silently, and with every name you check coming back plausible.

That is the difference between grounding and guessing which code you grounded.

### From § Step 0 (tail parenthetical, tier two)

(The `.worktrees/` name and the `wt new` that creates it are in Step 6. Current `wt` fetches before
resolving the base, but Step 6 still fetches explicitly — belt-and-suspenders, and required on older
builds — because brainstorm and planning take real time and the base moves while they do.)
