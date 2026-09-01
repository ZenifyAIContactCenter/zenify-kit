package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
)

// fakeGH records gh invocations and can create the clone destination on demand.
type fakeGH struct {
	calls   [][]string
	onClone func(dest string) error
}

func (f *fakeGH) Run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	// gh repo clone <org>/<name> <dest>
	if len(args) >= 4 && args[0] == "repo" && args[1] == "clone" && f.onClone != nil {
		return nil, f.onClone(args[3])
	}
	return nil, nil
}

// fakeGit is unused by wire/adopt (they touch the filesystem directly) but the
// signature must satisfy gitx.Runner for Apply.
type fakeGit struct{ calls [][]string }

func (g *fakeGit) Run(dir string, args ...string) ([]byte, error) {
	g.calls = append(g.calls, append([]string{dir}, args...))
	return nil, nil
}

// initBareRepoDir makes dir look like a clone: a .git/ with an info/ dir.
func initClonedRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o750); err != nil {
		t.Fatal(err)
	}
}

func TestApply_Clone_RunsGhAndWires(t *testing.T) {
	ws := t.TempDir()
	gh := &fakeGH{onClone: func(dest string) error {
		initClonedRepo(t, dest)
		return nil
	}}
	owned := &managed.Manifest{}
	res, err := Apply(
		[]reconcile.RepoPlan{{Name: "svc", State: reconcile.Clone, Path: "svc"}},
		Options{Workspace: ws, Org: "MyOrg", Owned: owned,
			RepoByName: map[string]manifest.Repo{"svc": {Name: "svc", Path: "svc"}}},
		gh, &fakeGit{},
	)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(gh.calls) != 1 || strings.Join(gh.calls[0], " ") != "repo clone MyOrg/svc "+filepath.Join(ws, "svc") {
		t.Fatalf("gh clone call = %v", gh.calls)
	}
	// After clone it wires: .git/info/exclude has .worktrees/, settings skeleton exists.
	assertExcludeHasWorktrees(t, filepath.Join(ws, "svc"))
	assertSettingsSkeleton(t, filepath.Join(ws, "svc"))
	if res[0].Skipped {
		t.Error("clone result should not be skipped")
	}
	if len(res[0].Wrote) == 0 {
		t.Error("clone+wire should record written files")
	}
}

func TestApply_Wire_Idempotent(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "svc")
	initClonedRepo(t, repo)
	owned := &managed.Manifest{}
	opts := Options{Workspace: ws, Owned: owned,
		RepoByName: map[string]manifest.Repo{"svc": {Name: "svc", Path: "svc"}}}
	plan := []reconcile.RepoPlan{{Name: "svc", State: reconcile.Wire, Path: "svc"}}

	if _, err := Apply(plan, opts, &fakeGH{}, &fakeGit{}); err != nil {
		t.Fatalf("first wire: %v", err)
	}
	// Second run must not duplicate the exclude line.
	if _, err := Apply(plan, opts, &fakeGH{}, &fakeGit{}); err != nil {
		t.Fatalf("second wire: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude")) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if strings.Count(string(b), ".worktrees/") != 1 {
		t.Errorf("exclude has %d .worktrees/ lines, want 1:\n%s", strings.Count(string(b), ".worktrees/"), b)
	}
}

func TestApply_Wire_PreservesExistingSettings(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "svc")
	initClonedRepo(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	existing := `{"env":{"SECRET":"keep-me"},"custom":true}`
	settingsPath := filepath.Join(repo, ".claude", "settings.local.json")
	if err := os.WriteFile(settingsPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	owned := &managed.Manifest{}
	_, err := Apply(
		[]reconcile.RepoPlan{{Name: "svc", State: reconcile.Wire, Path: "svc"}},
		Options{Workspace: ws, Owned: owned,
			RepoByName: map[string]manifest.Repo{"svc": {Name: "svc", Path: "svc"}}},
		&fakeGH{}, &fakeGit{},
	)
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	got, _ := os.ReadFile(settingsPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if string(got) != existing {
		t.Errorf("existing settings must be untouched (FR-041/FR-064d):\n got  %s\n want %s", got, existing)
	}
}

func TestApply_Wire_SeedsSecretKeyPlaceholders(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "svc")
	initClonedRepo(t, repo)
	owned := &managed.Manifest{}
	settingsPath := filepath.Join(repo, ".claude", "settings.local.json")
	_, err := Apply(
		[]reconcile.RepoPlan{{Name: "svc", State: reconcile.Wire, Path: "svc"}},
		Options{Workspace: ws, Owned: owned, SecretKeys: []string{"MONGO_URL", "E2E_EMAIL"},
			RepoByName: map[string]manifest.Repo{"svc": {Name: "svc", Path: "svc"}}},
		&fakeGH{}, &fakeGit{},
	)
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	env := readEnv(t, settingsPath)
	for _, k := range []string{"MONGO_URL", "E2E_EMAIL"} {
		v, ok := env[k]
		if !ok {
			t.Errorf("seeded env missing key %q:\n%v", k, env)
		}
		if v != "" {
			t.Errorf("placeholder %q must be empty, got %q (values are never distributed)", k, v)
		}
	}
	// A file the kit created in full is recorded for drift-tracking.
	if _, ok := owned.Get(settingsPath); !ok {
		t.Errorf("freshly created settings should be recorded in the ownership manifest")
	}
}

func TestApply_Wire_MergesMissingSecretKeys(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "svc")
	initClonedRepo(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(repo, ".claude", "settings.local.json")
	// MONGO_URL already has a value; custom is an unrelated top-level field.
	if err := os.WriteFile(settingsPath, []byte(`{"env":{"MONGO_URL":"keep-me"},"custom":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	owned := &managed.Manifest{}
	_, err := Apply(
		[]reconcile.RepoPlan{{Name: "svc", State: reconcile.Wire, Path: "svc"}},
		Options{Workspace: ws, Owned: owned, SecretKeys: []string{"MONGO_URL", "MYSQL_HOST"},
			RepoByName: map[string]manifest.Repo{"svc": {Name: "svc", Path: "svc"}}},
		&fakeGH{}, &fakeGit{},
	)
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	env := readEnv(t, settingsPath)
	if env["MONGO_URL"] != "keep-me" {
		t.Errorf("existing value must be preserved, MONGO_URL = %q, want keep-me", env["MONGO_URL"])
	}
	if v, ok := env["MYSQL_HOST"]; !ok || v != "" {
		t.Errorf("missing key must be added as empty placeholder, MYSQL_HOST = %q ok=%v", v, ok)
	}
	// Unrelated top-level fields survive the merge.
	var root map[string]any
	b, _ := os.ReadFile(settingsPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatalf("merged settings not valid JSON: %v", err)
	}
	if root["custom"] != true {
		t.Errorf("unrelated top-level field lost: %v", root)
	}
	// The kit does not own a pre-existing file, so a merge must not record it.
	if _, ok := owned.Get(settingsPath); ok {
		t.Errorf("a merged (pre-existing) settings file must NOT be recorded as kit-owned")
	}
}

func TestApply_Wire_MergePreservesValueBytes_NoHTMLEscape(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "svc")
	initClonedRepo(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(repo, ".claude", "settings.local.json")
	// A realistic Mongo URL: option separator "&" must survive the merge verbatim.
	mongo := "mongodb://h:27017/db?replicaSet=rs0&authSource=admin"
	if err := os.WriteFile(settingsPath, []byte(`{"env":{"MONGO_URL":"`+mongo+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Apply(
		[]reconcile.RepoPlan{{Name: "svc", State: reconcile.Wire, Path: "svc"}},
		Options{Workspace: ws, Owned: &managed.Manifest{}, SecretKeys: []string{"MONGO_URL", "MYSQL_HOST"},
			RepoByName: map[string]manifest.Repo{"svc": {Name: "svc", Path: "svc"}}},
		&fakeGH{}, &fakeGit{},
	)
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	raw, _ := os.ReadFile(settingsPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if !strings.Contains(string(raw), "&authSource") {
		t.Errorf("literal & must be preserved (no HTML-escape):\n%s", raw)
	}
	if strings.Contains(string(raw), `\u0026`) {
		t.Errorf("value bytes were HTML-escaped to the \\u0026 form, corrupting the dev's value:\n%s", raw)
	}
	// And the decoded value is still exactly what the dev wrote.
	if got := readEnv(t, settingsPath)["MONGO_URL"]; got != mongo {
		t.Errorf("MONGO_URL = %q, want %q", got, mongo)
	}
}

func TestApply_Wire_InvalidExistingSettings_SurfacesError(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "svc")
	initClonedRepo(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(repo, ".claude", "settings.local.json")
	garbage := []byte(`{not json`)
	if err := os.WriteFile(settingsPath, garbage, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Apply(
		[]reconcile.RepoPlan{{Name: "svc", State: reconcile.Wire, Path: "svc"}},
		Options{Workspace: ws, Owned: &managed.Manifest{}, SecretKeys: []string{"MONGO_URL"},
			RepoByName: map[string]manifest.Repo{"svc": {Name: "svc", Path: "svc"}}},
		&fakeGH{}, &fakeGit{},
	)
	if err != nil {
		t.Fatalf("Apply itself should not fail (per-repo error is captured): %v", err)
	}
	if res[0].Err == nil {
		t.Error("an unparseable settings file must surface as the repo's error, not be silently ignored")
	}
	// The unparseable file must be left exactly as it was.
	got, _ := os.ReadFile(settingsPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if string(got) != string(garbage) {
		t.Errorf("invalid file must be left untouched:\n got %s", got)
	}
}

func TestApply_Adopt_RecordsExistingWithoutModifying(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "svc")
	initClonedRepo(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(repo, ".claude", "settings.local.json")
	existing := `{"env":{"SECRET":"live-value"},"note":"adopt me"}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	owned := &managed.Manifest{}
	_, err := Apply(
		[]reconcile.RepoPlan{{Name: "svc", State: reconcile.Adopt, Path: "svc"}},
		Options{Workspace: ws, Owned: owned,
			RepoByName: map[string]manifest.Repo{"svc": {Name: "svc", Path: "svc"}}},
		&fakeGH{}, &fakeGit{},
	)
	if err != nil {
		t.Fatalf("Apply adopt: %v", err)
	}
	// The existing settings must be byte-for-byte unchanged (FR-041).
	got, _ := os.ReadFile(settingsPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if string(got) != existing {
		t.Errorf("adopt must not modify existing settings:\n got  %s\n want %s", got, existing)
	}
	// And it must be recorded in the ownership manifest for drift-tracking.
	if _, ok := owned.Get(settingsPath); !ok {
		t.Errorf("adopt should record existing settings in the ownership manifest")
	}
}

func TestApply_NoopStates_DoNothing(t *testing.T) {
	ws := t.TempDir()
	gh := &fakeGH{}
	plans := []reconcile.RepoPlan{
		{Name: "a", State: reconcile.SkipDirty, Path: "a"},
		{Name: "b", State: reconcile.WrongRemote, Path: "b"},
		{Name: "c", State: reconcile.NoAccess, Path: "c"},
		{Name: "d", State: reconcile.OK, Path: "d"},
		{Name: "e", State: reconcile.Migrate, Path: "e"},
	}
	res, err := Apply(plans, Options{Workspace: ws, Owned: &managed.Manifest{},
		RepoByName: map[string]manifest.Repo{}}, gh, &fakeGit{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(gh.calls) != 0 {
		t.Errorf("no-op states must not call gh, got %v", gh.calls)
	}
	for _, r := range res {
		if !r.Skipped {
			t.Errorf("state %s should be skipped", r.State)
		}
	}
}

func assertExcludeHasWorktrees(t *testing.T, repo string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude")) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(b), ".worktrees/") {
		t.Errorf("exclude missing .worktrees/:\n%s", b)
	}
}

// readEnv reads settings.local.json and returns its env block as string→string.
func readEnv(t *testing.T, settingsPath string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(settingsPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var root struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatalf("settings not valid JSON: %v\n%s", err, b)
	}
	return root.Env
}

func assertSettingsSkeleton(t *testing.T, repo string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, ".claude", "settings.local.json")) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var v map[string]json.RawMessage
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("settings not valid JSON: %v", err)
	}
	if _, ok := v["env"]; !ok {
		t.Errorf("settings skeleton missing env block:\n%s", b)
	}
}
