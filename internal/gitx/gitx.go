// Package gitx is an adapter over `git`: per-repo working-tree state and
// remote-URL normalization (read-only), plus worktree-linkage repair used by
// migrate (the one mutating operation, RepairWorktree).
package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner abstracts invoking `git -C <dir>` so tests can inject a fake.
type Runner interface {
	Run(dir string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(dir string, args ...string) ([]byte, error) {
	full := append([]string{"-C", dir}, args...)
	return exec.Command("git", full...).Output() //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
}

// ExecRunner returns the default Runner that shells to `git`.
func ExecRunner() Runner { return execRunner{} }

// RepoState is the read-only picture of one repo on disk.
type RepoState struct {
	Cloned           bool
	Path             string
	RemoteURL        string
	NormalizedRemote string
	Branch           string
	Dirty            bool
	DeployBranch     bool
	HasClaude        bool
	Layout           string // "none" | "old" (ignores .claude) | "new" (commits .claude)
}

var deployBaseline = map[string]bool{
	"main": true, "master": true, "staging": true, "develop": true,
	"develop-2": true, "development-2": true, "production": true,
}

// NormalizeRemote applies insteadOf rewrites, then reduces any github URL form
// (ssh, ssh-alias host, or https) to canonical "owner/repo".
func NormalizeRemote(url string, insteadOf map[string]string) string {
	u := strings.TrimSpace(url)
	for from, to := range insteadOf {
		if strings.HasPrefix(u, from) {
			u = to + strings.TrimPrefix(u, from)
		}
	}
	u = strings.TrimSuffix(u, ".git")
	// https://host/owner/repo
	if i := strings.Index(u, "://"); i >= 0 {
		rest := u[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			return rest[j+1:]
		}
		return rest
	}
	// git@host:owner/repo  (host may be an ssh-config alias)
	if i := strings.Index(u, ":"); i >= 0 {
		return u[i+1:]
	}
	return u
}

// Scan reads the state of the repo at dir. A dir without a .git entry is
// reported as not-cloned; everything else is read via read-only git commands.
func Scan(r Runner, dir string) (RepoState, error) {
	st := RepoState{Path: dir}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return st, nil // not cloned
	}
	st.Cloned = true

	if b, err := r.Run(dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		st.Branch = strings.TrimSpace(string(b))
	}
	st.DeployBranch = deployBaseline[st.Branch] || perRepoDeploy(dir, st.Branch)

	if b, err := r.Run(dir, "status", "--porcelain"); err == nil {
		st.Dirty = strings.TrimSpace(string(b)) != ""
	}

	if b, err := r.Run(dir, "remote", "get-url", "origin"); err == nil {
		st.RemoteURL = strings.TrimSpace(string(b))
		st.NormalizedRemote = NormalizeRemote(st.RemoteURL, readInsteadOf(r, dir))
	}

	if _, err := os.Stat(filepath.Join(dir, ".claude")); err == nil {
		st.HasClaude = true
	}
	st.Layout = detectLayout(dir)
	return st, nil
}

// HasWorktrees reports whether the repo at dir has any linked worktree
// beyond its main checkout.
func HasWorktrees(r Runner, dir string) (bool, error) {
	out, err := r.Run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return false, err
	}
	count := 0
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(ln, "worktree ") {
			count++
		}
	}
	return count > 1, nil
}

// ListWorktrees returns the paths of every linked worktree of the repo at dir,
// excluding the main checkout itself. Parsed from `git worktree list --porcelain`.
func ListWorktrees(r Runner, dir string) ([]string, error) {
	out, err := r.Run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var paths []string
	main := filepath.Clean(dir)
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		main = filepath.Clean(resolved) // git resolves symlinks (e.g. macOS /var -> /private/var) in its output
	}
	for _, ln := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(ln, "worktree ") {
			continue
		}
		p := strings.TrimSpace(strings.TrimPrefix(ln, "worktree "))
		if p == "" || filepath.Clean(p) == main {
			continue
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// RepairWorktree runs `git -C dir worktree repair path`, fixing the absolute-path
// linkage of a worktree after its main checkout (or the worktree) has moved.
// The bare form (no path) does not fix a moved worktree — the path is required.
func RepairWorktree(r Runner, dir, path string) error {
	_, err := r.Run(dir, "worktree", "repair", path)
	return err
}

// readInsteadOf collects url.<base>.insteadOf rewrites configured in the repo.
func readInsteadOf(r Runner, dir string) map[string]string {
	m := map[string]string{}
	b, err := r.Run(dir, "config", "--get-regexp", `url\..*\.insteadof`)
	if err != nil {
		return m
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		// "url.git@github.com:.insteadof git@github-zenify:"
		fields := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(fields) != 2 {
			continue
		}
		key := fields[0] // url.<to>.insteadof
		to := strings.TrimSuffix(strings.TrimPrefix(key, "url."), ".insteadof")
		from := strings.TrimSpace(fields[1])
		if from != "" && to != "" {
			m[from] = to
		}
	}
	return m
}

func perRepoDeploy(dir, branch string) bool {
	b, err := os.ReadFile(filepath.Join(dir, ".claude", "deploy-branches")) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == branch && branch != "" {
			return true
		}
	}
	return false
}

// detectLayout classifies the .gitignore: "new" if it commits .claude (only
// ignores secret/local files), "old" if it ignores /.claude wholesale, else "none".
func detectLayout(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, ".gitignore")) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		return "none"
	}
	text := string(b)
	for _, line := range strings.Split(text, "\n") {
		l := strings.TrimSpace(line)
		if l == ".claude" || l == "/.claude" || l == ".claude/" || l == "/.claude/" {
			return "old"
		}
	}
	if strings.Contains(text, "settings.local.json") {
		return "new"
	}
	return "none"
}
