// Package gitstate reports the git state of every repository in scope for the
// SessionStart and Stop hooks. It is the Go port of the personal
// session-git-state.sh: read-only, never blocks, exceptions-only output.
package gitstate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Mode selects the report shape: Session prints the full <git-state> block,
// Stop prints only the standing deploy-branch alarm.
type Mode int

const (
	Session Mode = iota
	Stop
)

const (
	scanDepth = 3  // ~/WorkingSpace/<container>/repos/<repo>/.git
	maxRepos  = 40 // a runaway scan is a hung session, not a slow one
)

var baseline = []string{"main", "master", "production", "staging", "develop"}

// RepoState is one repository's observed state.
type RepoState struct {
	Name     string
	Branch   string // "HEAD" when detached
	Dirty    int    // lines of `git status --porcelain`
	OnDeploy bool
	BaseRef  string // baseRef from .claude/worktree.json, "" when absent
	Behind   int    // local <base> commits behind BaseRef (Session mode only)
}

// gitTimeout bounds each shell-out so a hung git process (e.g. a wedged
// network fetch behind the scenes, or a lock held by another process) turns
// into a skipped repo, not a hung hook / hung session.
const gitTimeout = 5 * time.Second

func git(dir string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // G204 -- fixed binary, args are internal constants + paths
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return strings.TrimSpace(out.String()), true
}

// Scope returns the repo we are in, else the repos base contains (depth ≤3,
// skipping Library/ and node_modules/, capped at maxRepos, sorted). $HOME is
// refused: walking it yields ~177k files. Linked worktrees are excluded by
// the depth cap, not by any special-casing: a repo's own `<repo>/.worktrees/<name>`
// sits at depth 3, which hits SkipDir (below) before the walk ever descends
// into it looking for a `.git`. Raising scanDepth would pull those linked
// worktrees back in as separate repos — that is not a free change.
func Scope(base string) []string {
	base = filepath.Clean(base)
	if home, err := os.UserHomeDir(); err == nil && home != "" && filepath.Clean(home) == base {
		return nil
	}
	if fi, err := os.Stat(base); err != nil || !fi.IsDir() {
		return nil
	}
	if top, ok := git(base, "rev-parse", "--show-toplevel"); ok && top != "" {
		return []string{top}
	}
	var repos []string
	_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		if rel == "." {
			return nil
		}
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		name := d.Name()
		if name == "node_modules" || (depth == 1 && name == "Library") {
			return filepath.SkipDir
		}
		if name == ".git" {
			repos = append(repos, filepath.Dir(p))
			return filepath.SkipDir
		}
		if depth >= scanDepth {
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(repos)
	if len(repos) > maxRepos {
		repos = repos[:maxRepos]
	}
	return repos
}

// DeployPatterns is the union of every `.claude/deploy-branches` from repo up
// to stopAt (inclusive; "" walks to the filesystem root), comments and blank
// lines dropped, in leaf-to-root order as read. When no tier declares one,
// the baseline applies.
func DeployPatterns(repo, stopAt string) []string {
	var out []string
	found := false
	d := filepath.Clean(repo)
	stop := filepath.Clean(stopAt)
	for {
		if raw, err := os.ReadFile(filepath.Join(d, ".claude", "deploy-branches")); err == nil { //nolint:gosec // G304 -- fixed relative name under a repo path
			found = true
			sc := bufio.NewScanner(bytes.NewReader(raw))
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				out = append(out, line)
			}
		}
		parent := filepath.Dir(d)
		if (stopAt != "" && d == stop) || parent == d {
			break
		}
		d = parent
	}
	if !found {
		return append([]string(nil), baseline...)
	}
	return out
}

// IsDeploy matches branch against shell-style globs (`release*`, `prod-*`);
// `*` matches `/` too, mirroring bash `[[ $b == $pattern ]]`.
func IsDeploy(branch string, patterns []string) bool {
	if branch == "" || branch == "HEAD" {
		return false
	}
	for _, p := range patterns {
		p = strings.ReplaceAll(p, " ", "")
		if p == "" {
			continue
		}
		re := "^" + strings.NewReplacer(`\*`, ".*", `\?`, ".").Replace(regexp.QuoteMeta(p)) + "$"
		if ok, _ := regexp.MatchString(re, branch); ok {
			return true
		}
	}
	return false
}

// Inspect reads one repo. Stop mode skips the base-behind check (it runs per
// turn) and returns as soon as the tree is known clean.
func Inspect(repo string, mode Mode) RepoState {
	st := RepoState{Name: filepath.Base(repo)}
	branch, ok := git(repo, "rev-parse", "--abbrev-ref", "HEAD")
	if !ok || branch == "" {
		return st
	}
	st.Branch = branch
	if status, ok := git(repo, "status", "--porcelain"); ok && status != "" {
		st.Dirty = len(strings.Split(status, "\n"))
	}
	if mode == Stop {
		return st
	}
	raw, err := os.ReadFile(filepath.Join(repo, ".claude", "worktree.json")) //nolint:gosec // G304 -- fixed relative name under a repo path
	if err != nil {
		return st
	}
	var cfg struct {
		BaseRef string `json:"baseRef"`
	}
	if json.Unmarshal(raw, &cfg) != nil || !strings.HasPrefix(cfg.BaseRef, "origin/") {
		return st
	}
	st.BaseRef = cfg.BaseRef
	local := "refs/heads/" + strings.TrimPrefix(cfg.BaseRef, "origin/")
	if _, ok := git(repo, "show-ref", "--verify", "--quiet", local); !ok {
		return st
	}
	if n, ok := git(repo, "rev-list", "--count", local+".."+cfg.BaseRef); ok {
		st.Behind, _ = strconv.Atoi(n)
	}
	return st
}

// Run is what the hook calls: scope from base, deploy tiers up to stopAt,
// then Report. Empty scope => "".
func Run(base, stopAt string, mode Mode) string {
	repos := Scope(base)
	if len(repos) == 0 {
		return ""
	}
	states := make([]RepoState, 0, len(repos))
	for _, r := range repos {
		st := Inspect(r, mode)
		if st.Branch == "" {
			continue
		}
		if st.Dirty > 0 || mode == Session {
			st.OnDeploy = IsDeploy(st.Branch, DeployPatterns(r, stopAt))
		}
		states = append(states, st)
	}
	return Report(states, mode)
}
