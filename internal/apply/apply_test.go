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
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755); err != nil {
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
	b, _ := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if strings.Count(string(b), ".worktrees/") != 1 {
		t.Errorf("exclude has %d .worktrees/ lines, want 1:\n%s", strings.Count(string(b), ".worktrees/"), b)
	}
}

func TestApply_Wire_PreservesExistingSettings(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "svc")
	initClonedRepo(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"env":{"SECRET":"keep-me"},"custom":true}`
	settingsPath := filepath.Join(repo, ".claude", "settings.local.json")
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
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
	got, _ := os.ReadFile(settingsPath)
	if string(got) != existing {
		t.Errorf("existing settings must be untouched (FR-041/FR-064d):\n got  %s\n want %s", got, existing)
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
	b, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(b), ".worktrees/") {
		t.Errorf("exclude missing .worktrees/:\n%s", b)
	}
}

func assertSettingsSkeleton(t *testing.T, repo string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, ".claude", "settings.local.json"))
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
