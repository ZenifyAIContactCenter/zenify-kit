package wt

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// Row is one line of `wt ls`. All fields are render-ready strings; JSON tags are
// the editor-agnostic contract for `wt ls --json`.
type Row struct {
	Slug    string `json:"slug"`
	Branch  string `json:"branch"`
	Port    string `json:"port"`
	Deps    string `json:"deps"`
	Merged  string `json:"merged"`
	Running string `json:"running"`
	Path    string `json:"path"`
}

// List joins the repo's live worktrees (git = source of truth for existence)
// with their recorded annotations (state.json port first, then git-config), and
// derives MERGED (BranchMerged) and RUNNING (port bind-test) per worktree. The
// main checkout and worktrees outside worktreeDir are excluded.
func List(r gitx.Runner, repoRoot string, cfg *Config) ([]Row, error) {
	out, err := r.Run(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	st, err := ReadState(repoRoot)
	if err != nil {
		return nil, err
	}
	portBySlug := map[string]int{}
	portByPath := map[string]int{}
	for _, w := range st.Worktrees {
		if len(w.Ports) > 0 {
			portBySlug[w.Slug] = w.Ports[0]
			portByPath[w.Path] = w.Ports[0]
		}
	}
	// git reports canonical paths (on macOS the repo root resolves through
	// /private/var), while repoRoot may still carry an unresolved symlink; canon
	// both sides so the "inside worktreeDir" filter compares like with like.
	canonRoot := canon(repoRoot)
	wtPrefix := filepath.Join(canonRoot, cfg.WorktreeDir) + string(filepath.Separator)

	var rows []Row
	var curPath, curBranch string
	flush := func() []Row {
		defer func() { curPath, curBranch = "", "" }()
		cp := canon(curPath)
		if curPath == "" || cp == canonRoot || !strings.HasPrefix(cp, wtPrefix) {
			return nil
		}
		slug := cfgGet(r, curPath, "wt.slug")
		if slug == "" {
			slug = "(unmanaged)"
		}
		branch := curBranch
		merged := "-"
		if branch != "" && BranchMerged(r, repoRoot, branch, cfg.BaseRef) {
			merged = "merged"
		}
		if branch == "" {
			branch = "(detached)"
		}
		// port: state.json (by slug or by path) first, then git-config wt.port.
		port := "-"
		pnum := 0
		if p, ok := portBySlug[slug]; ok {
			pnum, port = p, strconv.Itoa(p)
		} else if p, ok := portByPath[curPath]; ok {
			pnum, port = p, strconv.Itoa(p)
		} else if gp := cfgGet(r, curPath, "wt.port"); gp != "" {
			port = gp
			if n, e := strconv.Atoi(gp); e == nil {
				pnum = n
			}
		}
		deps := cfgGet(r, curPath, "wt.deps")
		if deps == "" {
			deps = "-"
		}
		running := "-"
		if pnum > 0 && !portFree(pnum) {
			running = "running"
		}
		return []Row{{Slug: slug, Branch: branch, Port: port, Deps: deps, Merged: merged, Running: running, Path: curPath}}
	}
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			rows = append(rows, flush()...)
			curPath = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			curBranch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "":
			rows = append(rows, flush()...)
		}
	}
	rows = append(rows, flush()...)
	return rows, nil
}

// canon resolves symlinks in p (so /var vs /private/var comparisons match),
// falling back to p unchanged when the path does not exist yet.
func canon(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// cfgGet reads a worktree-scoped git config value with dir=worktreePath, or ""
// on any error / empty value.
func cfgGet(r gitx.Runner, worktreePath, key string) string {
	b, err := r.Run(worktreePath, "config", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// URLFor returns http://localhost:<port> for slug, resolved through the same
// join List uses. Errors when the slug is not found or has no numeric port.
func URLFor(r gitx.Runner, repoRoot string, cfg *Config, slug string) (string, error) {
	rows, err := List(r, repoRoot, cfg)
	if err != nil {
		return "", err
	}
	for _, row := range rows {
		if row.Slug == slug {
			if row.Port == "-" {
				return "", fmt.Errorf("wt: worktree %q has no recorded port", slug)
			}
			return "http://localhost:" + row.Port, nil
		}
	}
	return "", fmt.Errorf("wt: no worktree %q", slug)
}
