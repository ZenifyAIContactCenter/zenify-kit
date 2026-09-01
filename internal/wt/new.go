package wt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// NewOptions is the fully-resolved input to RunNew. Pid/Now/Host are injected so
// the core stays deterministic; the cobra command fills them from the runtime.
type NewOptions struct {
	RepoRoot     string
	Slug         string
	Type         string
	BaseOverride string
	Host         string
	ForceInstall bool
	Another      bool
	Pid          int
	Now          int64
	Runner       gitx.Runner
	Stderr       io.Writer
}

// validSlug allows lowercase letters, digits and dashes only (matching bash wt).
func validSlug(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

func validType(t string) bool {
	switch t {
	case "feat", "fix", "chore", "hotfix":
		return true
	}
	return false
}

// RunNew creates a worktree for o.Slug end-to-end. Everything that can fail is
// resolved BEFORE `git worktree add` touches disk; any failure after the add is
// followed by abort cleanup so a failed run leaves nothing behind.
func RunNew(o NewOptions) error {
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	if !validSlug(o.Slug) {
		return fmt.Errorf("wt: slug must be lowercase letters, digits and dashes")
	}
	if !validType(o.Type) {
		return fmt.Errorf("wt: type must be feat, fix, chore or hotfix")
	}
	r := o.Runner
	cfg, err := Load(o.RepoRoot)
	if err != nil {
		return err
	}
	user := cfg.User
	branch := fmt.Sprintf("%s/%s/%s", user, o.Type, o.Slug)
	path := filepath.Join(o.RepoRoot, cfg.WorktreeDir, o.Slug)

	// base: explicit override > hotfixBaseRef (hotfix only) > configured baseRef.
	base := cfg.BaseRef
	if o.BaseOverride != "" {
		base = o.BaseOverride
	} else if o.Type == "hotfix" {
		if cfg.HotfixBaseRef != "" {
			base = cfg.HotfixBaseRef
		} else {
			fmt.Fprintf(o.Stderr, "wt: no hotfixBaseRef declared — branching this hotfix from %s\n", base)
		}
	}

	// Duplicate checks (keyed on the exact slug/branch).
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("wt: task %q already exists at %s", o.Slug, path)
	}
	if _, err := r.Run(o.RepoRoot, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		return fmt.Errorf("wt: task %q already exists as branch %s", o.Slug, branch)
	}

	// Guard tier 1: one task per repo per session (self-healing stale pointer).
	ptr, active := SessionPtr(o.RepoRoot)
	if active && o.Type != "hotfix" && !o.Another {
		if prev, e := os.ReadFile(ptr); e == nil {
			prevSlug := strings.TrimSpace(string(prev))
			if prevSlug != "" && prevSlug != o.Slug {
				prevPath := filepath.Join(o.RepoRoot, cfg.WorktreeDir, prevSlug)
				if _, e := os.Stat(prevPath); e == nil {
					return fmt.Errorf("wt: this session already opened task %q in this repo\n  continue there:   cd %s\n  separate task:    wt new %s --another\n  production fix:   wt new %s --type hotfix", prevSlug, prevPath, o.Slug, o.Slug)
				}
				os.Remove(ptr) // stale: recorded worktree is gone
			}
		}
	}

	// Guard tier 2: no second UNMERGED task of this user in the repo (beyond session).
	if active && o.Type != "hotfix" && !o.Another {
		open, err := RepoOpenTasks(r, o.RepoRoot, user, cfg.WorktreeDir, base)
		if err != nil {
			return err
		}
		if len(open) > 0 {
			var b strings.Builder
			fmt.Fprintf(&b, "wt: %s already has unmerged tasks of yours:\n", filepath.Base(o.RepoRoot))
			for _, t := range open {
				fmt.Fprintf(&b, "       %-34s %s\n", t.Slug, t.Branch)
			}
			fmt.Fprintf(&b, "opening %q as well needs --another — or tear one down first: wt rm <slug>", o.Slug)
			return fmt.Errorf("%s", b.String())
		}
	}

	// base ref must exist.
	if _, err := r.Run(o.RepoRoot, "rev-parse", "--verify", "--quiet", base); err != nil {
		return fmt.Errorf("wt: base ref %q not found — fetch first", base)
	}

	// Allocate the port up front (a full range fails before touching disk).
	st, err := ReadState(o.RepoRoot)
	if err != nil {
		return err
	}
	taken := map[int]bool{}
	for _, w := range st.Worktrees {
		for _, p := range w.Ports {
			taken[p] = true
		}
	}
	key := fmt.Sprintf("%s:%s:%s", filepath.Base(o.RepoRoot), o.Slug, cfg.PortEnv)
	port, ok := Allocate(key, cfg.PortRange[0], cfg.PortRange[1], taken)
	if !ok {
		return fmt.Errorf("wt: no free port in %v", cfg.PortRange)
	}

	deps := cfg.Deps
	if o.ForceInstall {
		deps = "install"
	}

	// Point of no return: create the worktree, then abort-clean on any failure.
	if _, err := r.Run(o.RepoRoot, "worktree", "add", "-q", path, "-b", branch, base); err != nil {
		return fmt.Errorf("wt: git worktree add failed: %w", err)
	}
	abort := func(cause error) error {
		// Best-effort cleanup, but not silent: if cleanup itself fails, a
		// half-built worktree or dangling branch survives, and the next
		// `wt new <same-slug>` would trip the duplicate check with no visible
		// reason. Warn so the leftover is at least diagnosable.
		if _, e := r.Run(o.RepoRoot, "worktree", "remove", "--force", path); e != nil {
			fmt.Fprintf(o.Stderr, "wt: warning — could not remove worktree %s during cleanup; remove it by hand: %v\n", path, e)
		}
		if _, e := r.Run(o.RepoRoot, "branch", "-D", branch); e != nil {
			fmt.Fprintf(o.Stderr, "wt: warning — could not delete branch %s during cleanup; delete it by hand: %v\n", branch, e)
		}
		return cause
	}

	if _, err := r.Run(o.RepoRoot, "config", "extensions.worktreeConfig", "true"); err != nil {
		return abort(fmt.Errorf("wt: could not enable extensions.worktreeConfig: %w", err))
	}

	warns, err := SeedCopyFiles(o.RepoRoot, path, cfg.Copy)
	if err != nil {
		return abort(err)
	}
	for _, w := range warns {
		fmt.Fprintf(o.Stderr, "wt: warning — %s\n", w)
	}

	envPath := filepath.Join(path, orEnvFile(cfg))
	if err := WritePortEnv(envPath, cfg.PortEnv, port); err != nil {
		return abort(fmt.Errorf("wt: write %s: %w", cfg.PortEnv, err))
	}
	if err := SeedIdentityEnv(envPath, cfg, o.Slug); err != nil {
		return abort(fmt.Errorf("wt: seed identity env: %w", err))
	}

	if err := ApplyDeps(o.RepoRoot, path, deps, "node_modules", cfg.Install); err != nil {
		return abort(fmt.Errorf("wt: deps setup failed: %w", err))
	}

	// Record worktree-scoped git config so `wt ls`/guards can rebuild from git.
	for _, kv := range [][2]string{{"wt.slug", o.Slug}, {"wt.type", o.Type}, {"wt.port", fmt.Sprintf("%d", port)}, {"wt.deps", deps}} {
		if _, err := r.Run(path, "config", "--worktree", kv[0], kv[1]); err != nil {
			return abort(fmt.Errorf("wt: could not record %s: %w", kv[0], err))
		}
	}

	// Persist to the rebuildable caches (state + global index).
	wtRec := Worktree{Slug: o.Slug, Type: o.Type, Branch: branch, Path: path, Ports: []int{port}}
	if err := SaveWorktree(o.RepoRoot, wtRec, o.Pid, o.Host, o.Now); err != nil {
		return abort(err)
	}
	if err := IndexUpsert(o.RepoRoot, o.Slug, o.Pid, o.Host, o.Now); err != nil {
		return abort(err)
	}

	// Only past every abort: record the session pointer (a failed run leaves none).
	if active {
		os.WriteFile(ptr, []byte(o.Slug+"\n"), 0o644)
	}

	fmt.Fprintf(o.Stderr, "wt: %s → %s\n", o.Slug, path)
	fmt.Fprintf(o.Stderr, "wt: branch %s, %s=%d, deps=%s\n", branch, cfg.PortEnv, port, deps)
	return nil
}

// orEnvFile returns the configured envFile or the default ".env".
func orEnvFile(c *Config) string {
	if strings.TrimSpace(c.EnvFile) != "" {
		return c.EnvFile
	}
	return ".env"
}
