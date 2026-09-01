package wt

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// SessionPtr returns the pointer file for this session+repo and whether the
// one-task-per-repo guard is active. A set-but-EMPTY WT_SESSION means "no
// session, guard off" (how tests disable it); unset means the same. The pointer
// lives under XDG_CACHE_HOME (stable across the sandbox's TMPDIR rewrite), keyed
// by session id, with the repo's absolute path flattened (/ → _) into the name.
func SessionPtr(repoRoot string) (string, bool) {
	sess, ok := os.LookupEnv("WT_SESSION")
	if !ok || sess == "" {
		return "", false
	}
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		base = filepath.Join(home, ".cache")
	}
	dir := filepath.Join(base, "wt", "session-"+sess)
	name := strings.ReplaceAll(repoRoot, "/", "_")
	return filepath.Join(dir, name), true
}

// BranchMerged reports whether branch's work is already in base: either its tip
// is an ancestor of base, or (squash-merge) `git diff base..branch` is empty.
// wt ls and wt rm must both ask exactly this so they never disagree.
func BranchMerged(r gitx.Runner, repoRoot, branch, base string) bool {
	if branch == "" || base == "" {
		return false
	}
	if _, err := r.Run(repoRoot, "merge-base", "--is-ancestor", branch, base); err == nil {
		return true
	}
	out, err := r.Run(repoRoot, "diff", base+".."+branch)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == ""
}

// OpenTask is one unmerged worktree belonging to the querying user.
type OpenTask struct {
	Slug   string
	Branch string
}

// RepoOpenTasks lists every worktree in repoRoot on an unmerged branch owned by
// user (branch prefixed "<user>/"), as the second-tier guard for `wt new`. Two
// filters, both deliberate: unmerged-only (finished work awaiting teardown does
// not block a new task) and this-user-only (a colleague's worktree never trips
// the guard).
func RepoOpenTasks(r gitx.Runner, repoRoot, user, worktreeDir, base string) ([]OpenTask, error) {
	out, err := r.Run(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	// Trailing separator on purpose: filepath.Join strips worktreeDir's slash, so
	// a bare HasPrefix would also match a sibling like ".worktrees-backup/" whose
	// name merely starts with the same string. Requiring the separator forces a
	// full path-segment boundary — only paths genuinely inside the dir match.
	wtPrefix := filepath.Join(repoRoot, worktreeDir) + string(filepath.Separator)
	var tasks []OpenTask
	var curPath, curBranch string
	flush := func() {
		defer func() { curPath, curBranch = "", "" }()
		if curPath == "" || curPath == repoRoot {
			return
		}
		if !strings.HasPrefix(curPath, wtPrefix) {
			return
		}
		if curBranch == "" || !strings.HasPrefix(curBranch, user+"/") {
			return
		}
		if BranchMerged(r, repoRoot, curBranch, base) {
			return
		}
		slug := "(unmanaged)"
		// Runner.Run takes the working dir as its first arg, so read the
		// worktree's own config with dir=curPath rather than a `-C` flag.
		if s, e := r.Run(curPath, "config", "--get", "wt.slug"); e == nil {
			if v := strings.TrimSpace(string(s)); v != "" {
				slug = v
			}
		}
		tasks = append(tasks, OpenTask{Slug: slug, Branch: curBranch})
	}
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			curPath = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			curBranch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "":
			flush()
		}
	}
	flush()
	return tasks, nil
}
