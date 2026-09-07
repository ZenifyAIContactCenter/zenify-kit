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
	"time"

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

	// FR-9a/b: bật per-repo transaction khi SnapshotRoot != "". Rỗng cả hai =
	// hành vi legacy (apply + Record in-memory, caller tự Save) để test cũ
	// trong apply_test.go (không set các field này) chạy nguyên.
	SnapshotRoot string                                     // .zenify/snapshots — nơi chứa per-repo snapshot
	ManifestPath string                                     // .zenify/manifest.json — Save sau mỗi repo promote
	Now          func() int64                               // clock cho snapshot id (nil → time.Now)
	VerifyRepoFn func(repoDir string, wrote []string) error // nil → defaultVerifyRepo
}

func (o Options) now() int64 {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now().Unix()
}

func (o Options) verify(repoDir string, wrote []string) error {
	if o.VerifyRepoFn != nil {
		return o.VerifyRepoFn(repoDir, wrote)
	}
	return defaultVerifyRepo(wrote)
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

		if !isActionable(p.State) {
			// OK, DRIFT:skip-dirty, wrong-remote, SKIP:no-access, SKIP,
			// MIGRATE-layout (automated flip deferred to M2): report, do nothing.
			r.Skipped = true
			r.Action = "skipped (" + string(p.State) + ")"
			results = append(results, r)
			continue
		}

		transact := opts.SnapshotRoot != ""
		repoFiles := []string{
			filepath.Join(repoDir, ".claude", "settings.local.json"),
			filepath.Join(repoDir, ".git", "info", "exclude"),
		}

		var (
			snapDir       string
			existedBefore map[string]bool
			priorKeys     map[string]struct{}
		)
		if transact {
			existedBefore = statSet(repoFiles)
			priorKeys = ownedKeySet(opts.Owned)
			id := fmt.Sprintf("txn-%s-%d", p.Name, opts.now())
			sd, serr := managed.Snapshot(id, repoFiles, opts.SnapshotRoot)
			if serr != nil {
				r.Err = fmt.Errorf("snapshot %s: %w", p.Name, serr)
				results = append(results, r)
				continue
			}
			snapDir = sd
		}

		r.Action, r.Wrote, r.Err = applyOne(p, repoDir, opts, gh, git)
		if r.Err == nil && transact {
			r.Err = opts.verify(repoDir, r.Wrote)
		}

		if transact {
			if r.Err != nil {
				revertRepo(snapDir, repoFiles, existedBefore)
				revertOwnedKeys(opts.Owned, priorKeys)
			} else if opts.ManifestPath != "" {
				if serr := opts.Owned.Save(opts.ManifestPath); serr != nil {
					r.Err = fmt.Errorf("promote %s: %w", p.Name, serr)
				}
			}
		}
		results = append(results, r)
	}
	return results, nil
}

// applyOne chạy đúng thao tác ghi theo state (nội dung switch cũ). gh/git chỉ
// dùng cho Clone; Wire/Adopt không đụng tới.
func applyOne(p reconcile.RepoPlan, repoDir string, opts Options, gh ghx.Runner, git gitx.Runner) (string, []string, error) {
	switch p.State {
	case reconcile.Clone:
		if err := cloneRepo(gh, opts.Org, p.Name, repoDir); err != nil {
			return "clone + wire", nil, err
		}
		wrote, err := wireRepo(repoDir, opts.Owned, opts.SecretKeys)
		return "clone + wire", wrote, err
	case reconcile.Wire:
		wrote, err := wireRepo(repoDir, opts.Owned, opts.SecretKeys)
		return "wire config", wrote, err
	case reconcile.Adopt:
		wrote, err := adoptRepo(repoDir, opts.Owned)
		return "adopt in place", wrote, err
	default:
		return "skipped (" + string(p.State) + ")", nil, nil
	}
}

func isActionable(s reconcile.State) bool {
	return s == reconcile.Clone || s == reconcile.Wire || s == reconcile.Adopt
}

// defaultVerifyRepo kiểm mỗi file repo vừa ghi tồn tại + parse được:
// settings.local.json là JSON có "env" object; .git/info/exclude chứa dòng
// .worktrees/. Chỉ kiểm các file trong `wrote` (thứ repo này thực sự đụng).
func defaultVerifyRepo(wrote []string) error {
	for _, f := range wrote {
		switch {
		case strings.HasSuffix(f, "settings.local.json"):
			b, err := os.ReadFile(f) //nolint:gosec // G304 -- path computed internally from workspace/plan, not externally-tainted
			if err != nil {
				return fmt.Errorf("verify %s: %w", f, err)
			}
			var root map[string]any
			if err := json.Unmarshal(b, &root); err != nil {
				return fmt.Errorf("verify %s: not valid JSON: %w", f, err)
			}
			if _, ok := root["env"].(map[string]any); !ok {
				return fmt.Errorf("verify %s: missing env object", f)
			}
		case strings.HasSuffix(f, filepath.Join("info", "exclude")):
			b, err := os.ReadFile(f) //nolint:gosec // G304 -- path computed internally from workspace/plan, not externally-tainted
			if err != nil {
				return fmt.Errorf("verify %s: %w", f, err)
			}
			found := false
			for _, ln := range strings.Split(string(b), "\n") {
				if strings.TrimSpace(ln) == ".worktrees/" {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("verify %s: missing .worktrees/ line", f)
			}
		}
	}
	return nil
}

// statSet trả tập file đã tồn tại trong danh sách.
func statSet(files []string) map[string]bool {
	m := map[string]bool{}
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			m[f] = true
		}
	}
	return m
}

// revertRepo hoàn nguyên đúng phạm vi repo này: restore file đã tồn tại-từ-trước
// về nội dung snapshot, rồi xoá file repo này MỚI tạo (không có trong snapshot
// vì managed.Snapshot bỏ qua file vắng — nên Restore không tự xoá chúng).
func revertRepo(snapDir string, repoFiles []string, existedBefore map[string]bool) {
	if snapDir != "" {
		_ = managed.Restore(snapDir)
	}
	for _, f := range repoFiles {
		if existedBefore[f] {
			continue
		}
		if _, err := os.Stat(f); err == nil {
			_ = os.Remove(f)
		}
	}
}

// ownedKeySet chụp tập key manifest trước khi apply một repo.
func ownedKeySet(m *managed.Manifest) map[string]struct{} {
	s := map[string]struct{}{}
	if m == nil {
		return s
	}
	for k := range m.Entries {
		s[k] = struct{}{}
	}
	return s
}

// revertOwnedKeys bỏ mọi key được Record in-memory trong repo vừa fail (key
// không có trong ảnh chụp trước-apply), để Save của repo sau không persist nhầm.
func revertOwnedKeys(m *managed.Manifest, prior map[string]struct{}) {
	if m == nil {
		return
	}
	for k := range m.Entries {
		if _, ok := prior[k]; !ok {
			delete(m.Entries, k)
		}
	}
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
	if err := os.MkdirAll(infoDir, 0o750); err != nil {
		return "", err
	}
	excludePath := filepath.Join(infoDir, "exclude")
	b, err := os.ReadFile(excludePath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
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
	if err := os.WriteFile(excludePath, []byte(content), 0o600); err != nil { //nolint:gosec // G703 -- excludePath is the repo's own .git/info/exclude, computed internally, not externally-tainted input
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

	b, err := os.ReadFile(settingsPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	switch {
	case err == nil:
		return mergeSettingsKeys(settingsPath, b, secretKeys)
	case !os.IsNotExist(err):
		return "", err
	}

	// Absent — create with the required keys as empty placeholders.
	if err := os.MkdirAll(claudeDir, 0o750); err != nil {
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
