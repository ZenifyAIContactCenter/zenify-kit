// Package workspace locates git repos in a workspace by depth, so a consumer
// doesn't have to assume "repo = direct child of root". Pure: readDir is injected.
package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

const DefaultMaxDepth = 2

// Repo is one git repo found. Name = basename, Path = path to the repo directory.
type Repo struct {
	Name string
	Path string
}

// isRepo: a directory containing a ".git" entry that is a DIRECTORY (main checkout), OR
// containing ".claude/worktree.json". A linked worktree has ".git" as a file → not counted
// as a repo. Reads go through the injected readDir instead of calling os.Stat directly, to
// keep the function pure/testable.
func isRepo(dir string, entries []os.DirEntry, readDir func(string) ([]os.DirEntry, error)) bool {
	for _, e := range entries {
		if e.Name() == ".git" && e.IsDir() {
			return true
		}
	}
	hasClaudeDir := false
	for _, e := range entries {
		if e.Name() == ".claude" && e.IsDir() {
			hasClaudeDir = true
			break
		}
	}
	if !hasClaudeDir {
		return false
	}
	claudeEntries, err := readDir(filepath.Join(dir, ".claude"))
	if err != nil {
		return false
	}
	for _, e := range claudeEntries {
		if e.Name() == "worktree.json" && !e.IsDir() {
			return true
		}
	}
	return false
}

// Discover walks from root down to maxDepth; once a branch is recognized as a repo, it
// STOPS recursing into that branch. Skips hidden dirs (.git, .worktrees, .claude) and
// node_modules so it never wanders inside a repo.
func Discover(root string, maxDepth int, readDir func(string) ([]os.DirEntry, error)) []Repo {
	var out []Repo
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}
		entries, err := readDir(dir)
		if err != nil {
			return
		}
		if depth >= 1 && isRepo(dir, entries, readDir) {
			out = append(out, Repo{Name: filepath.Base(dir), Path: dir})
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" {
				continue
			}
			walk(filepath.Join(dir, e.Name()), depth+1)
		}
	}
	walk(root, 0)
	return out
}

// Resolve finds a repo by name; if the name collides across multiple places, returns the
// shallowest one (fewest path segments).
func Resolve(root, name string, maxDepth int, readDir func(string) ([]os.DirEntry, error)) (string, bool) {
	best := ""
	bestDepth := 1 << 30
	for _, r := range Discover(root, maxDepth, readDir) {
		if r.Name != name {
			continue
		}
		d := strings.Count(r.Path, string(filepath.Separator))
		if d < bestDepth {
			best, bestDepth = r.Path, d
		}
	}
	return best, best != ""
}
