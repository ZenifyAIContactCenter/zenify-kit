package wt

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// SweepItem is one worktree's sweep verdict.
type SweepItem struct {
	Slug   string
	Branch string
	Port   string
	Path   string
	Reason string
	Remove bool
}

// sweepPlan decides, per worktree in the repo, whether it is safe to tear down.
// A worktree is removable only if it was created by wt (has wt.slug), is on a
// real branch (not detached), that branch has a merge trace in base, and its
// tree is clean. Everything else is kept, with the reason recorded. Pure: reads
// git only through r, no side effects — this is the unit-tested core of sweep.
func sweepPlan(r gitx.Runner, repoRoot, base, worktreeDir string) ([]SweepItem, error) {
	out, err := r.Run(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	wtPrefix := filepath.Join(canon(repoRoot), worktreeDir) + string(filepath.Separator)

	var items []SweepItem
	var curPath string
	flush := func() {
		defer func() { curPath = "" }()
		if curPath == "" || !strings.HasPrefix(canon(curPath), wtPrefix) {
			return
		}
		slug := cfgGet(r, curPath, "wt.slug")
		it := SweepItem{Slug: slug, Path: curPath}
		if slug == "" {
			it.Reason = "not created by wt"
			items = append(items, it)
			return
		}
		branch := ""
		if b, e := r.Run(curPath, "symbolic-ref", "--quiet", "--short", "HEAD"); e == nil {
			branch = strings.TrimSpace(string(b))
		}
		it.Branch = branch
		it.Port = cfgGet(r, curPath, "wt.port")
		if branch == "" {
			it.Reason = "detached HEAD"
			items = append(items, it)
			return
		}
		if !BranchMerged(r, repoRoot, branch, base) {
			it.Reason = fmt.Sprintf("no merge trace in %s", base)
			items = append(items, it)
			return
		}
		if cfgStatusDirty(r, curPath) {
			it.Reason = "merged but uncommitted changes"
			items = append(items, it)
			return
		}
		it.Remove = true
		it.Reason = "merged, clean"
		items = append(items, it)
	}
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			curPath = strings.TrimPrefix(line, "worktree ")
		case line == "":
			flush()
		}
	}
	flush()
	return items, nil
}

// cfgStatusDirty reports whether the worktree at worktreePath has uncommitted
// changes (git status --porcelain non-empty).
func cfgStatusDirty(r gitx.Runner, worktreePath string) bool {
	out, _ := r.Run(worktreePath, "status", "--porcelain")
	return strings.TrimSpace(string(out)) != ""
}
