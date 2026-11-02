package wt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitguard"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// RmOptions is the fully-resolved input to RunRm. Pid/Now/Host are injected so
// the core stays deterministic (they flow into RemoveWorktree).
type RmOptions struct {
	RepoRoot string
	Slug     string
	Host     string
	Force    bool
	Fetch    bool // refresh origin before the merge check (the CLI sets it; sweep has fetched already)
	// KeepRemote leaves origin/<branch> alone. wt new's auto-sweep sets it: an empty
	// branch counts as merged (guard.go BranchMerged), so an untouched worktree of
	// another task would otherwise lose the remote branch that marks it started.
	KeepRemote bool
	Pid        int
	Now        int64
	Runner     gitx.Runner
	// RemoteRunner runs the network calls (ls-remote/fetch/push); the CLI passes a
	// TimeoutRunner. Nil falls back to Runner.
	RemoteRunner gitx.Runner
	Stderr       io.Writer
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
		if o.Fetch {
			if _, e := r.Run(o.RepoRoot, "fetch", "origin", "--quiet"); e != nil {
				_, _ = fmt.Fprintf(o.Stderr, "wt: fetch failed (%v) — merge state may be stale\n", e)
			}
		}
		if ok, checked := WorktreeMerged(r, o.RepoRoot, path, branch, base); !ok {
			return fmt.Errorf("wt: %q (%s) has no merge trace in %s — if it was squash-merged, re-run with --force", o.Slug, branch, checked)
		}
	}

	// What wt recorded for this slug, read BEFORE the worktree and state go away:
	// the remote branch is only ever deleted if it is the very branch wt created,
	// counted against the base that worktree was cut from.
	recBranch := ""
	if st, e := ReadState(o.RepoRoot); e == nil {
		if w, ok := st.Find(o.Slug); ok {
			recBranch = w.Branch
		}
	}
	remoteBase := base
	if !gone {
		if b := cfgGet(r, path, "wt.base"); b != "" {
			remoteBase = b
		}
	}

	// Remove the worktree (git-level --force: our gate already vouched for it).
	if gone {
		if _, e := r.Run(o.RepoRoot, "worktree", "prune"); e != nil {
			return fmt.Errorf("wt: git worktree prune failed: %w", e)
		}
	} else if _, e := r.Run(o.RepoRoot, "worktree", "remove", "--force", path); e != nil {
		// An orphan: the directory is there but git no longer registers it — its
		// .git file points at a gitdir that moved (seen after `zenify migrate`
		// relocated a repo with worktrees still inside). git refuses to remove
		// what it does not know, so delete the directory ourselves and prune.
		// Anything git DOES register stays a hard error.
		out, le := r.Run(o.RepoRoot, "worktree", "list", "--porcelain")
		if le != nil || worktreeListHasPath(string(out), path) {
			return fmt.Errorf("wt: git worktree remove failed: %w", e)
		}
		if re := os.RemoveAll(path); re != nil {
			return fmt.Errorf("wt: %s is not registered in git and could not be deleted: %w", path, re)
		}
		_, _ = fmt.Fprintf(o.Stderr, "wt: %s was not registered in git (%v) — directory deleted directly\n", path, e)
		if _, pe := r.Run(o.RepoRoot, "worktree", "prune"); pe != nil {
			_, _ = fmt.Fprintf(o.Stderr, "wt:   worktree prune failed: %v\n", pe)
		}
		// The orphan checkout could not answer symbolic-ref; state.json still
		// knows which branch wt created for it.
		if branch == "" {
			if st, se := ReadState(o.RepoRoot); se == nil {
				if w, ok := st.Find(o.Slug); ok {
					branch = w.Branch
				}
			}
		}
	}

	if branch == "" {
		_, _ = fmt.Fprintf(o.Stderr, "wt: %q was on a detached HEAD — no branch to delete\n", o.Slug)
	} else if _, e := r.Run(o.RepoRoot, "branch", "-D", branch); e != nil {
		_, _ = fmt.Fprintf(o.Stderr, "wt: warning — could not delete branch %s, delete it by hand: %v\n", branch, e)
	} else if !o.KeepRemote {
		deleteEmptyRemote(o, branch, recBranch, remoteBase, cfg.BaseRef)
	}

	// Clear the rebuildable caches. Best-effort: a cache-write failure must not
	// leave the (already removed) worktree looking un-removed to the user.
	if _, e := RemoveWorktree(o.RepoRoot, o.Slug, o.Pid, o.Host, o.Now); e != nil {
		_, _ = fmt.Fprintf(o.Stderr, "wt: warning — could not update state.json: %v\n", e)
	}

	// Release the session pointer if it names the slug just removed, so the next
	// task in this repo is not refused in favour of one that is gone.
	if ptr, active := SessionPtr(o.RepoRoot); active {
		if b, e := os.ReadFile(ptr); e == nil && strings.TrimSpace(string(b)) == o.Slug { //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
			_ = os.Remove(ptr)
		}
	}

	_, _ = fmt.Fprintf(o.Stderr, "wt: removed %q\n", o.Slug)

	// Peers pointing at this repo fall back to their baseline now that the
	// worktree is gone (RunWire finds no same-slug worktree here).
	if ws, ok := FindWorkspaceRoot(o.RepoRoot); ok {
		RewirePeers(ws, filepath.Base(o.RepoRoot), o.Slug, r, o.Stderr, o.Stderr)
	}
	return nil
}

// deleteEmptyRemote removes origin/<branch> only when (1) branch is the one wt
// recorded for this slug — a worktree switched to main must never take main off
// origin — (2) it is not a base or deploy branch, and (3) its tip has no commit
// beyond base (the empty branch wt new pushed). A branch with commits is kept.
// Every failure only warns; no origin remote means nothing to do.
func deleteEmptyRemote(o RmOptions, branch, recorded, base, cfgBase string) {
	r := o.Runner
	rr := remoteOr(o.RemoteRunner, r)
	if _, e := r.Run(o.RepoRoot, "remote", "get-url", "origin"); e != nil {
		return
	}
	if branch != recorded || isProtectedBranch(o.RepoRoot, branch, base, cfgBase) {
		_, _ = fmt.Fprintf(o.Stderr, "wt: kept origin/%s: not a wt task branch\n", branch)
		return
	}
	out, e := rr.Run(o.RepoRoot, "ls-remote", "--heads", "origin", branch)
	if e != nil {
		_, _ = fmt.Fprintf(o.Stderr, "wt: warning — could not look up origin/%s: %s\n", branch, gitErrLine(e))
		return
	}
	tip := ""
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == "refs/heads/"+branch {
			tip = f[0]
		}
	}
	if tip == "" {
		return
	}
	if _, e := rr.Run(o.RepoRoot, "fetch", "origin", "--quiet", "refs/heads/"+branch); e != nil {
		_, _ = fmt.Fprintf(o.Stderr, "wt: warning — could not fetch origin/%s, keeping it: %s\n", branch, gitErrLine(e))
		return
	}
	cnt, e := r.Run(o.RepoRoot, "rev-list", "--count", base+".."+tip)
	if e != nil {
		_, _ = fmt.Fprintf(o.Stderr, "wt: warning — could not count commits on origin/%s, keeping it: %s\n", branch, gitErrLine(e))
		return
	}
	if strings.TrimSpace(string(cnt)) != "0" {
		_, _ = fmt.Fprintf(o.Stderr, "wt: kept origin/%s: it has commits\n", branch)
		return
	}
	// Lease on the tip we counted: a commit pushed since makes git refuse, so we
	// can never delete work that arrived after the check.
	if _, e := rr.Run(o.RepoRoot, "push", "--force-with-lease=refs/heads/"+branch+":"+tip, "origin", "--delete", branch); e != nil {
		_, _ = fmt.Fprintf(o.Stderr, "wt: warning — could not delete origin/%s, keeping it: %s\n", branch, gitErrLine(e))
		return
	}
	_, _ = fmt.Fprintf(o.Stderr, "wt: deleted empty origin/%s\n", branch)
}

// isProtectedBranch is true for a base branch (either the base the worktree was
// cut from or the configured one, short name), the baseline deploy names, and
// anything the repo's .claude/deploy-branches lists.
func isProtectedBranch(repoRoot, branch string, bases ...string) bool {
	for _, b := range bases {
		if branch == strings.TrimPrefix(b, "origin/") {
			return true
		}
	}
	return baselineDeploy[branch] || gitguard.IsDeployBranch(repoRoot, branch)
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
