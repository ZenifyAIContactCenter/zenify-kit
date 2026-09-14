package cli

import (
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

// sourcesScanDepth is how deep the Where step looks under the user's "parent
// of my clones" directory. 3 covers <parent>/<group>/<repo> layouts.
const sourcesScanDepth = 3

// scanSources finds existing clones of manifest repos under dir, matched by
// origin remote (owner/repo, insteadOf-aware) — never by directory name
// (FR-3.5). Each hit carries whether it has attached worktrees (FR-4.2).
func scanSources(m *manifest.Manifest, git gitx.Runner, dir string) map[string]reconcile.Source {
	out := map[string]reconcile.Source{}
	if dir == "" {
		return out
	}
	want := map[string]string{} // owner/repo → manifest name
	for _, r := range m.Repos {
		// manifest URLs are canonical on purpose — no insteadOf applied here;
		// the scanned side goes through the real insteadOf in gitx.Scan below.
		want[gitx.NormalizeRemote(r.URL, nil)] = r.Name
	}
	for _, rp := range workspace.Discover(dir, sourcesScanDepth, os.ReadDir) {
		st, err := gitx.Scan(git, rp.Path)
		if err != nil || !st.Cloned || st.NormalizedRemote == "" {
			continue
		}
		name, ok := want[st.NormalizedRemote]
		if !ok {
			continue
		}
		if _, dup := out[name]; dup {
			continue // first hit wins; a second clone of the same repo is left alone
		}
		abs, _ := filepath.Abs(rp.Path)
		has, _ := gitx.HasWorktrees(git, rp.Path)
		out[name] = reconcile.Source{Path: abs, HasWorktrees: has}
	}
	return out
}

// flatSources treats the legacy flat layout <ws>/<name> as a source when the
// manifest path is empty (FR-4.1), so `up` relocates it instead of cloning twice.
func flatSources(m *manifest.Manifest, git gitx.Runner, ws string) map[string]reconcile.Source {
	out := map[string]reconcile.Source{}
	for _, r := range m.Repos {
		dest := filepath.Join(ws, r.Path)
		flat := filepath.Join(ws, r.Name)
		if filepath.Clean(dest) == filepath.Clean(flat) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(flat, ".git")); err != nil {
			continue
		}
		has, _ := gitx.HasWorktrees(git, flat)
		out[r.Name] = reconcile.Source{Path: flat, HasWorktrees: has}
	}
	return out
}
