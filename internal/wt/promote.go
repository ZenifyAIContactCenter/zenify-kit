package wt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// PromoteOptions drives RunPromote.
type PromoteOptions struct {
	RepoRoot string
	Slug     string
	Runner   gitx.Runner
	Stdout   io.Writer
	Stderr   io.Writer
}

// RunPromote turns a worktree's SYMLINKED node_modules into a private CoW copy,
// so the task can diverge its dependencies from the main checkout. Drop-in port
// of bash cmd_promote: three lstat outcomes (already-private → no-op; no deps at
// all → refuse; symlink → replace with a copy), then record wt.deps=clone and
// install. The copy landing before the bookkeeping write is deliberate — a
// failed git-config write is a warning, not a rolled-back promote.
func RunPromote(o PromoteOptions) error {
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	r := o.Runner
	cfg, err := Load(o.RepoRoot)
	if err != nil {
		return err
	}
	st, err := ReadState(o.RepoRoot)
	if err != nil {
		return err
	}
	w, ok := st.Find(o.Slug)
	if !ok {
		return fmt.Errorf("wt: no worktree %q", o.Slug)
	}
	path := w.Path
	if path == "" {
		return fmt.Errorf("wt: worktree %q has no path in state", o.Slug)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(o.RepoRoot, path)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("wt: no worktree for %q at %s", o.Slug, path)
	}

	const nm = "node_modules"
	dst := filepath.Join(path, nm)
	src := filepath.Join(o.RepoRoot, nm)

	fi, lerr := os.Lstat(dst)
	// Exists and is NOT a symlink → already a private tree; nothing to do.
	if lerr == nil && fi.Mode()&os.ModeSymlink == 0 {
		_, _ = fmt.Fprintf(o.Stdout, "wt: %q already has a private %s — nothing to do\n", o.Slug, nm)
		return nil
	}
	// Not a symlink (and not the private case above) → there is nothing to promote.
	if lerr != nil {
		return fmt.Errorf("wt: %q has no %s at all — nothing to promote; install in %s first (or re-create the task with deps=install)", o.Slug, nm, path)
	}

	// It is a symlink: replace it with a real CoW copy of the main checkout's tree.
	if err := os.Remove(dst); err != nil {
		return fmt.Errorf("wt: could not remove the %s symlink: %w", nm, err)
	}
	if err := copyTree(src, dst); err != nil {
		return fmt.Errorf("wt: copy failed — %q had its symlink removed and now has no %s at all: %w", o.Slug, nm, err)
	}

	// The copy has landed. A failed bookkeeping write is a WARNING, not a failure
	// of the promote — reporting it as failure would invite a re-run that hits the
	// already-private branch and never records anything.
	if _, err := r.Run(path, "config", "--worktree", "wt.deps", "clone"); err != nil {
		_, _ = fmt.Fprintf(o.Stderr, "wt: %q now has a private %s — the copy succeeded\n", o.Slug, nm)
		_, _ = fmt.Fprintf(o.Stderr, "wt: warning — could not record wt.deps=clone; 'wt ls' will show deps as '-' until you run:\n")
		_, _ = fmt.Fprintf(o.Stderr, "      git -C %s config --worktree wt.deps clone\n", path)
		if ierr := runInstall(path, cfg.Install); ierr != nil {
			return fmt.Errorf("wt: install failed in %s: %w", path, ierr)
		}
		return fmt.Errorf("wt: promote of %q completed but wt.deps bookkeeping was not recorded", o.Slug)
	}
	_, _ = fmt.Fprintf(o.Stdout, "wt: %q promoted to a private %s\n", o.Slug, nm)
	if err := runInstall(path, cfg.Install); err != nil {
		return fmt.Errorf("wt: install failed in %s: %w", path, err)
	}
	return nil
}
