package wt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// RmOptions is the fully-resolved input to RunRm. Pid/Now/Host are injected so
// the core stays deterministic (they flow into RemoveWorktree/IndexRemove).
type RmOptions struct {
	RepoRoot string
	Slug     string
	Host     string
	Force    bool
	Pid      int
	Now      int64
	Runner   gitx.Runner
	Stderr   io.Writer
}

// RunRm tears down the worktree for o.Slug. Without --force it refuses a dirty
// tree, a detached HEAD, or a branch with no merge trace in base — so it can
// only ever remove work that has landed. It then removes the worktree, deletes
// the branch, and clears the rebuildable caches (state.json, global index) and
// the session pointer.
func RunRm(o RmOptions) error {
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	r := o.Runner
	cfg, err := Load(o.RepoRoot)
	if err != nil {
		return err
	}
	path := filepath.Join(o.RepoRoot, cfg.WorktreeDir, o.Slug)
	base := cfg.BaseRef

	gone := false
	if _, e := os.Stat(path); e != nil {
		gone = true
	}

	// Resolve the branch: from the checkout when it exists, else from state.json
	// (which recorded it at wt new time) so a hand-deleted dir can still be cleaned.
	// An existing checkout directory is itself proof of registration; a gone dir is
	// only cleanable if the slug was actually a worktree once — confirmed from the
	// caches git can rebuild from (state.json, then git's own registration, which
	// outlives a hand-deleted directory until `worktree prune`).
	branch := ""
	registered := !gone
	if gone {
		if st, e := ReadState(o.RepoRoot); e == nil {
			for _, w := range st.Worktrees {
				if w.Slug == o.Slug {
					branch, registered = w.Branch, true
					break
				}
			}
		}
		if !registered {
			if out, e := r.Run(o.RepoRoot, "worktree", "list", "--porcelain"); e == nil && worktreeListHasPath(string(out), path) {
				registered = true
			}
		}
		// A slug that was never a worktree (a typo, or one never created) must not
		// fall through to a repo-wide `worktree prune` and a false "removed" report.
		// Mirrors bash cmd_rm's `wt_registration || die`; --force cannot bypass it.
		if !registered {
			return fmt.Errorf("wt: no worktree %q — nothing at %s and no registration to clean up", o.Slug, path)
		}
	} else {
		if b, e := r.Run(path, "symbolic-ref", "--quiet", "--short", "HEAD"); e == nil {
			branch = strings.TrimSpace(string(b))
		}
	}

	if !o.Force {
		if gone {
			return fmt.Errorf("wt: %q has no checkout left to inspect — re-run with --force to drop its registration and branch", o.Slug)
		}
		if out, _ := r.Run(path, "status", "--porcelain"); strings.TrimSpace(string(out)) != "" {
			return fmt.Errorf("wt: %q has uncommitted changes — commit them, or re-run with --force", o.Slug)
		}
		if branch == "" {
			return fmt.Errorf("wt: %q is on a detached HEAD — wt cannot tell whether its commits are safe; re-run with --force", o.Slug)
		}
		if !BranchMerged(r, o.RepoRoot, branch, base) {
			return fmt.Errorf("wt: %q (%s) has no merge trace in %s — if it was squash-merged, re-run with --force", o.Slug, branch, base)
		}
	}

	// Remove the worktree (git-level --force: our gate already vouched for it).
	if gone {
		if _, e := r.Run(o.RepoRoot, "worktree", "prune"); e != nil {
			return fmt.Errorf("wt: git worktree prune failed: %w", e)
		}
	} else {
		if _, e := r.Run(o.RepoRoot, "worktree", "remove", "--force", path); e != nil {
			return fmt.Errorf("wt: git worktree remove failed: %w", e)
		}
	}

	if branch == "" {
		_, _ = fmt.Fprintf(o.Stderr, "wt: %q was on a detached HEAD — no branch to delete\n", o.Slug)
	} else if _, e := r.Run(o.RepoRoot, "branch", "-D", branch); e != nil {
		_, _ = fmt.Fprintf(o.Stderr, "wt: warning — could not delete branch %s, delete it by hand: %v\n", branch, e)
	}

	// Clear the rebuildable caches. Best-effort: a cache-write failure must not
	// leave the (already removed) worktree looking un-removed to the user.
	if _, e := RemoveWorktree(o.RepoRoot, o.Slug, o.Pid, o.Host, o.Now); e != nil {
		_, _ = fmt.Fprintf(o.Stderr, "wt: warning — could not update state.json: %v\n", e)
	}
	if e := IndexRemove(o.RepoRoot, o.Slug, o.Pid, o.Host, o.Now); e != nil {
		_, _ = fmt.Fprintf(o.Stderr, "wt: warning — could not update the global index: %v\n", e)
	}

	// Release the session pointer if it names the slug just removed, so the next
	// task in this repo is not refused in favour of one that is gone.
	if ptr, active := SessionPtr(o.RepoRoot); active {
		if b, e := os.ReadFile(ptr); e == nil && strings.TrimSpace(string(b)) == o.Slug { //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
			_ = os.Remove(ptr)
		}
	}

	_, _ = fmt.Fprintf(o.Stderr, "wt: removed %q\n", o.Slug)
	return nil
}

// worktreeListHasPath reports whether git's porcelain worktree listing registers
// a worktree at path. Both sides are canonicalized so a symlinked repo root (macOS
// /var vs /private/var) still matches git's canonical path.
func worktreeListHasPath(porcelain, path string) bool {
	cp := canon(path)
	for _, line := range strings.Split(porcelain, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			if canon(strings.TrimPrefix(line, "worktree ")) == cp {
				return true
			}
		}
	}
	return false
}
