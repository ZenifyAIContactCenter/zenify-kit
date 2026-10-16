# Stacked layers

Applies only when the plan has a `## Stack` block. Layer 1 is the `wt` branch (`git branch --show-current`); layer N >= 2 is `<layer-1 branch>-<N>`. Trunk is `baseRef` from `.claude/worktree.json` without the `origin/` prefix. One worktree serves the whole slug.

1. Before the first task of layer 1: `gh stack init --base <trunk> <layer-1 branch>`.
2. Before the first task of layer N >= 2: `gh stack add <layer-1 branch>-<N>`, without `-A`, `-u` or `-m`.
3. A task's BASE is `git rev-parse HEAD` on its own layer branch.
4. A fix that belongs to a lower layer k than the one you stand on: `gh stack checkout <layer-k branch>`, commit, `gh stack rebase --upstack`, then `gh stack checkout <current layer branch>`. Detect a conflict from git state, never from an exit code: `git status --porcelain` shows an unmerged line (`UU`, `AA`, `DU`, `UD`, `AU`, `UA`, `DD`), or the directory `$(git rev-parse --git-path rebase-merge)` exists. On a conflict run `gh stack rebase --abort`, ledger `BLOCKED: rebase conflict at layer <k>`, stop and tell the human. Never resolve it yourself, never push.
5. No interactive commands: `checkout` always names a branch; never `modify` or `switch`. Read state with `gh stack view --json`, not exit codes.
6. Ledger line: `Task <N>: complete (layer <L>, commits <base7>..<head7>, review clean)`.
