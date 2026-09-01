// Package apply executes the actionable states of a reconcile plan. It is the
// first caller of the b2a safety infra. It never commits to a repo, never
// switches branches, and never reads or copies a secret value: WIRE seeds only
// an empty settings skeleton and a local .git/info/exclude entry. worktree.json
// seeding is deferred to wt-v2 (it needs port-range allocation).
package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
)

// Options carry the shared inputs Apply needs beyond the plan itself.
type Options struct {
	Workspace  string                   // workspace root; repo paths are relative to it
	Org        string                   // GitHub org for `gh repo clone <org>/<name>`
	Owned      *managed.Manifest        // ownership manifest to Record written files into
	RepoByName map[string]manifest.Repo // manifest entry per repo name (for URL/base)
}

// Result is the outcome for one repo.
type Result struct {
	Repo    string
	State   reconcile.State
	Action  string   // human-readable summary
	Wrote   []string // files zenify wrote/recorded this run
	Skipped bool
	Err     error
}

// Apply executes each plan by its state. A per-repo error is captured in that
// repo's Result and does not abort the others; Apply returns a non-nil error
// only for a failure that makes continuing meaningless (currently none — the
// caller decides based on Result.Err). The order of plans is preserved.
func Apply(plans []reconcile.RepoPlan, opts Options, gh ghx.Runner, git gitx.Runner) ([]Result, error) {
	results := make([]Result, 0, len(plans))
	for _, p := range plans {
		r := Result{Repo: p.Name, State: p.State}
		repoDir := filepath.Join(opts.Workspace, p.Path)
		switch p.State {
		case reconcile.Clone:
			r.Action = "clone + wire"
			if err := cloneRepo(gh, opts.Org, p.Name, repoDir); err != nil {
				r.Err = err
				break
			}
			wrote, err := wireRepo(repoDir, opts.Owned)
			r.Wrote, r.Err = wrote, err
		case reconcile.Wire:
			r.Action = "wire config"
			wrote, err := wireRepo(repoDir, opts.Owned)
			r.Wrote, r.Err = wrote, err
		case reconcile.Adopt:
			r.Action = "adopt in place"
			wrote, err := adoptRepo(repoDir, opts.Owned)
			r.Wrote, r.Err = wrote, err
		default:
			// OK, DRIFT:skip-dirty, wrong-remote, SKIP:no-access, SKIP,
			// MIGRATE-layout (→ B2b-2): report, do nothing.
			r.Skipped = true
			r.Action = "skipped (" + string(p.State) + ")"
		}
		results = append(results, r)
	}
	return results, nil
}

// cloneRepo clones via gh, respecting the developer's configured gh protocol
// (OQ-9). gh repo clone creates the destination directory.
func cloneRepo(gh ghx.Runner, org, name, dest string) error {
	if org == "" {
		return fmt.Errorf("apply: empty org for clone of %q", name)
	}
	if _, err := gh.Run("repo", "clone", org+"/"+name, dest); err != nil {
		return fmt.Errorf("gh repo clone %s/%s: %w", org, name, err)
	}
	return nil
}

// wireRepo makes the local, non-committing config edits: a .git/info/exclude
// entry for .worktrees/ (OQ-5) and an empty settings.local.json skeleton if the
// repo has none. Existing settings are left byte-for-byte untouched (FR-064d,
// FR-041). Returns the paths written (for the caller's summary/manifest).
func wireRepo(repoDir string, owned *managed.Manifest) ([]string, error) {
	var wrote []string
	w, err := ensureExclude(repoDir)
	if err != nil {
		return wrote, err
	}
	if w != "" {
		wrote = append(wrote, w)
	}
	w, err = ensureSettingsSkeleton(repoDir, owned)
	if err != nil {
		return wrote, err
	}
	if w != "" {
		wrote = append(wrote, w)
	}
	return wrote, nil
}

// adoptRepo recognises an already-configured repo in place: it ensures the
// .worktrees/ exclude and records any managed file that already exists, without
// changing file contents, branches, or location (FR-013).
func adoptRepo(repoDir string, owned *managed.Manifest) ([]string, error) {
	var wrote []string
	w, err := ensureExclude(repoDir)
	if err != nil {
		return wrote, err
	}
	if w != "" {
		wrote = append(wrote, w)
	}
	settings := filepath.Join(repoDir, ".claude", "settings.local.json")
	if _, statErr := os.Stat(settings); statErr == nil {
		if err := owned.Record(settings); err != nil {
			return wrote, err
		}
		wrote = append(wrote, settings)
	}
	return wrote, nil
}

// ensureExclude appends ".worktrees/" to the repo's .git/info/exclude if it is
// not already present. Returns the exclude path if it was modified, "" if it
// already contained the entry. The .git/info directory is assumed to exist in a
// real clone; ensureExclude creates it if missing so a freshly cloned repo is
// covered.
func ensureExclude(repoDir string) (string, error) {
	infoDir := filepath.Join(repoDir, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return "", err
	}
	excludePath := filepath.Join(infoDir, "exclude")
	b, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == ".worktrees/" {
			return "", nil // already present
		}
	}
	content := string(b)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += ".worktrees/\n"
	if err := os.WriteFile(excludePath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return excludePath, nil
}

// ensureSettingsSkeleton writes .claude/settings.local.json as {"env":{}} only
// if it does not already exist, and records it in the ownership manifest. An
// existing file is never read for its values, never merged, never overwritten
// (that is B2b-3's job with the required-key list). Returns the path if written.
func ensureSettingsSkeleton(repoDir string, owned *managed.Manifest) (string, error) {
	claudeDir := filepath.Join(repoDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	if _, err := os.Stat(settingsPath); err == nil {
		return "", nil // present — leave it entirely alone
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return "", err
	}
	skeleton, err := json.MarshalIndent(map[string]any{"env": map[string]any{}}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(settingsPath, append(skeleton, '\n'), 0o644); err != nil {
		return "", err
	}
	if err := owned.Record(settingsPath); err != nil {
		return "", err
	}
	return settingsPath, nil
}
