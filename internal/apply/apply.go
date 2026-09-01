// Package apply executes the actionable states of a reconcile plan. It is the
// first caller of the b2a safety infra. It never commits to a repo, never
// switches branches, and never merges or copies a secret value: WIRE seeds only
// an empty settings skeleton, and ADOPT only fingerprints an existing file's
// bytes for drift-tracking (the content is never persisted or logged).
// worktree.json seeding is deferred to wt-v2 (it needs port-range allocation).
package apply

import (
	"bytes"
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
	SecretKeys []string                 // env keys to scaffold as empty placeholders (FR-065); values never distributed
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
			wrote, err := wireRepo(repoDir, opts.Owned, opts.SecretKeys)
			r.Wrote, r.Err = wrote, err
		case reconcile.Wire:
			r.Action = "wire config"
			wrote, err := wireRepo(repoDir, opts.Owned, opts.SecretKeys)
			r.Wrote, r.Err = wrote, err
		case reconcile.Adopt:
			r.Action = "adopt in place"
			wrote, err := adoptRepo(repoDir, opts.Owned)
			r.Wrote, r.Err = wrote, err
		default:
			// OK, DRIFT:skip-dirty, wrong-remote, SKIP:no-access, SKIP,
			// MIGRATE-layout (automated flip deferred to M2): report, do nothing.
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
// entry for .worktrees/ (OQ-5) and a settings.local.json seeded with the
// required env keys as empty placeholders (FR-065). An existing settings file
// keeps every value it already has; only missing keys are added (FR-064d).
// Returns the paths written (for the caller's summary/manifest).
func wireRepo(repoDir string, owned *managed.Manifest, secretKeys []string) ([]string, error) {
	var wrote []string
	// The exclude file is reported in Wrote (it changed on disk this run) but is
	// deliberately NOT Record'd into the ownership manifest: the kit owns only the
	// single ".worktrees/" line it appended, not the whole git-local file, so
	// fingerprinting the whole file would false-flag on any user edit. Only files
	// the kit owns in full (the settings skeleton) are Record'd.
	w, err := ensureExclude(repoDir)
	if err != nil {
		return wrote, err
	}
	if w != "" {
		wrote = append(wrote, w)
	}
	w, err = ensureSettingsSkeleton(repoDir, owned, secretKeys)
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

// ensureSettingsSkeleton makes .claude/settings.local.json carry every required
// env key (FR-065). It NEVER distributes, reads, logs, or overwrites a secret
// value — placeholders are empty strings and existing values are preserved
// verbatim (FR-041, FR-064d).
//
//   - Absent: create it as {"env":{<key>:"" ...}} and record it in the ownership
//     manifest (the kit owns a file it created in full).
//   - Present: add only the required keys that are missing, each as an empty
//     placeholder; keep every existing key, value, and top-level field. It is
//     NOT recorded — the kit does not own a file the developer already had.
//
// Returns the path if the file was created or modified, "" if nothing changed.
// A present file that is not valid JSON, or whose "env" is not an object, is
// left untouched and surfaces as an error so the developer can fix it (the run
// continues for other repos).
func ensureSettingsSkeleton(repoDir string, owned *managed.Manifest, secretKeys []string) (string, error) {
	claudeDir := filepath.Join(repoDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	b, err := os.ReadFile(settingsPath)
	switch {
	case err == nil:
		return mergeSettingsKeys(settingsPath, b, secretKeys)
	case !os.IsNotExist(err):
		return "", err
	}

	// Absent — create with the required keys as empty placeholders.
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return "", err
	}
	env := make(map[string]any, len(secretKeys))
	for _, k := range secretKeys {
		env[k] = ""
	}
	skeleton, err := json.MarshalIndent(map[string]any{"env": env}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := managed.WriteFileAtomic(settingsPath, append(skeleton, '\n')); err != nil {
		return "", err
	}
	if err := owned.Record(settingsPath); err != nil {
		return "", err
	}
	return settingsPath, nil
}

// mergeSettingsKeys adds the missing required keys (as empty placeholders) to an
// existing settings.local.json without touching any value already there. It
// re-marshals canonically (2-space indent, keys sorted by encoding/json), so an
// existing file may be reformatted, but no value is ever changed or removed.
func mergeSettingsKeys(settingsPath string, raw []byte, secretKeys []string) (string, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return "", fmt.Errorf("settings %s is not valid JSON, leaving it untouched: %w", settingsPath, err)
	}
	if root == nil {
		root = map[string]any{}
	}
	env, ok := root["env"].(map[string]any)
	if !ok {
		if _, present := root["env"]; present {
			return "", fmt.Errorf("settings %s has a non-object \"env\", leaving it untouched", settingsPath)
		}
		env = map[string]any{}
		root["env"] = env
	}
	changed := false
	for _, k := range secretKeys {
		if _, present := env[k]; !present {
			env[k] = ""
			changed = true
		}
	}
	if !changed {
		return "", nil
	}
	// Encode with HTML-escaping OFF so a developer's existing value keeps its exact
	// bytes: json.Marshal would rewrite "&", "<", ">" as & etc., which would
	// mangle e.g. the "&" separating options in a Mongo URL. Write atomically
	// (temp+rename, mode-preserving) so a crash mid-write can never truncate a
	// settings file that already holds live secret values.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(root); err != nil { // Encode appends a trailing newline
		return "", err
	}
	if err := managed.WriteFileAtomic(settingsPath, buf.Bytes()); err != nil {
		return "", err
	}
	return settingsPath, nil
}
