package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/lock"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
)

type fakeGH struct {
	auth, list []byte
	cloneErr   bool
}

func (f fakeGH) Run(args ...string) ([]byte, error) {
	if len(args) > 1 && args[0] == "repo" && args[1] == "clone" && f.cloneErr {
		return nil, fmt.Errorf("clone failed")
	}
	if len(args) > 1 && args[0] == "auth" {
		return f.auth, nil
	}
	return f.list, nil
}

type fakeGit struct{}

func (fakeGit) Run(dir string, args ...string) ([]byte, error) { return nil, nil }

func testManifest() *manifest.Manifest {
	return &manifest.Manifest{Org: "ZenifyAIContactCenter", Repos: []manifest.Repo{
		{Name: "contact-center-be", URL: "git@github.com:ZenifyAIContactCenter/contact-center-be.git", Path: "contact-center-be", Base: "origin/staging"},
	}}
}

func TestBuildPlanAuthAndClassify(t *testing.T) {
	gh := fakeGH{
		auth: []byte("  ✓ Logged in to github.com account natepxn\n  - Token scopes: 'read:org', 'repo'\n"),
		list: []byte(`[{"name":"contact-center-be","sshUrl":"git@github.com:ZenifyAIContactCenter/contact-center-be.git","viewerPermission":"MAINTAIN","isArchived":false}]`),
	}
	plans, auth, err := buildPlan(testManifest(), gh, fakeGit{}, t.TempDir())
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	if !auth.LoggedIn || !auth.HasScopes("read:org", "repo") {
		t.Errorf("auth = %+v", auth)
	}
	if len(plans) != 1 || plans[0].State != reconcile.Clone {
		// path does not exist under empty workspace → not cloned → CLONE
		t.Errorf("plans = %+v", plans)
	}
}

func TestBuildPlanNoAccess(t *testing.T) {
	gh := fakeGH{
		auth: []byte("  ✓ Logged in to github.com account x\n  - Token scopes: 'read:org', 'repo'\n"),
		list: []byte(`[]`), // sees no repos
	}
	plans, _, err := buildPlan(testManifest(), gh, fakeGit{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].State != reconcile.NoAccess {
		t.Errorf("State = %q, want NoAccess", plans[0].State)
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	plans := []reconcile.RepoPlan{{Name: "be", State: reconcile.Clone, Reason: "absent", Path: "be"}}
	if err := renderPlanJSON(&buf, plans, ghx.Auth{LoggedIn: true, Account: "x"}); err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.SchemaVersion != "1" {
		t.Errorf("schema = %q", env.SchemaVersion)
	}
	if !strings.Contains(buf.String(), "CLONE") {
		t.Errorf("json missing state:\n%s", buf.String())
	}
}

func TestBuildPlan_NotLoggedIn_ReturnsNilPlans(t *testing.T) {
	gh := fakeGH{
		auth: []byte("You are not logged into any GitHub hosts. Run gh auth login to authenticate.\n"),
		list: []byte(`[]`),
	}
	plans, auth, err := buildPlan(testManifest(), gh, fakeGit{}, t.TempDir())
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	if auth.LoggedIn {
		t.Errorf("expected LoggedIn=false, got %+v", auth)
	}
	if plans != nil {
		t.Errorf("expected nil plans when logged out, got %+v", plans)
	}
}

func TestRunApply_WiresRepoAndWritesManifest(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "svc")
	if err := os.MkdirAll(filepath.Join(repo, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Org: "MyOrg", Repos: []manifest.Repo{{Name: "svc", Path: "svc"}}}
	plans := []reconcile.RepoPlan{{Name: "svc", State: reconcile.Wire, Path: "svc"}}

	err := runApply(io.Discard, plans, m, ws, &fakeGH{}, &fakeGit{})
	if err != nil {
		t.Fatalf("runApply: %v", err)
	}
	// Wired: settings skeleton + exclude present.
	if _, err := os.Stat(filepath.Join(repo, ".claude", "settings.local.json")); err != nil {
		t.Errorf("settings skeleton not written: %v", err)
	}
	// Ownership manifest persisted under the workspace.
	if _, err := os.Stat(filepath.Join(ws, ".zenify", "manifest.json")); err != nil {
		t.Errorf("ownership manifest not saved: %v", err)
	}
}

func TestRunApply_LockHeld_ReturnsExit4(t *testing.T) {
	ws := t.TempDir()
	// Hold the lock, then a second runApply must map ErrHeld → exit 4.
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o755); err != nil {
		t.Fatal(err)
	}
	h, err := lock.Acquire(filepath.Join(ws, ".zenify"), os.Getpid(), "host", 1)
	if err != nil {
		t.Fatalf("pre-acquire (ensure .zenify exists first): %v", err)
	}
	defer h.Release()

	m := &manifest.Manifest{Org: "MyOrg", Repos: []manifest.Repo{}}
	err = runApply(io.Discard, nil, m, ws, &fakeGH{}, &fakeGit{})
	if exitcode.Code(err) != exitcode.LockHeld {
		t.Fatalf("want exit %d (LockHeld), got %d (err %v)", exitcode.LockHeld, exitcode.Code(err), err)
	}
}

func TestRunApply_PartialFailure_SavesManifestAndReturnsFail(t *testing.T) {
	ws := t.TempDir()
	// A CLONE plan whose gh clone fails → the repo's Result.Err is set.
	gh := &fakeGH{cloneErr: true}
	m := &manifest.Manifest{Org: "MyOrg", Repos: []manifest.Repo{{Name: "svc", Path: "svc"}}}
	plans := []reconcile.RepoPlan{{Name: "svc", State: reconcile.Clone, Path: "svc"}}

	err := runApply(io.Discard, plans, m, ws, gh, &fakeGit{})
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("want exit %d (Fail) on a failed repo, got %d (err %v)", exitcode.Fail, exitcode.Code(err), err)
	}
	// The ownership manifest must still be persisted (succeeded repos are recorded).
	if _, statErr := os.Stat(filepath.Join(ws, ".zenify", "manifest.json")); statErr != nil {
		t.Errorf("ownership manifest not saved after a partial failure: %v", statErr)
	}
}

func TestDryRunApplyConflict(t *testing.T) {
	// args: (apply, dryRunChanged, dryRun)
	// default: dry-run true by default but not explicitly set, no --apply → ok
	if err := dryRunApplyConflict(false, false, true); err != nil {
		t.Fatalf("default (no apply, dry-run unset): unexpected error %v", err)
	}
	// --apply alone (dry-run not explicitly set) → allowed
	if err := dryRunApplyConflict(true, false, true); err != nil {
		t.Fatalf("apply alone: unexpected error %v", err)
	}
	// explicit --dry-run (true) alone → allowed
	if err := dryRunApplyConflict(false, true, true); err != nil {
		t.Fatalf("explicit dry-run alone: unexpected error %v", err)
	}
	// --apply with an explicit --dry-run=false → coherent mutate request, allowed
	if err := dryRunApplyConflict(true, true, false); err != nil {
		t.Fatalf("apply + explicit --dry-run=false: unexpected error %v", err)
	}
	// --apply with an explicit --dry-run(=true) → contradiction, rejected
	if err := dryRunApplyConflict(true, true, true); err == nil {
		t.Fatal("apply + explicit --dry-run=true: expected a conflict error, got nil")
	}
}

func TestHasFrontendRepo(t *testing.T) {
	m := &manifest.Manifest{Repos: []manifest.Repo{
		{Name: "be", Tags: []string{"primary", "backend"}},
		{Name: "web", Tags: []string{"primary", "frontend"}},
	}}
	if !hasFrontendRepo(m) {
		t.Fatal("should detect the frontend repo")
	}
	m2 := &manifest.Manifest{Repos: []manifest.Repo{{Name: "be", Tags: []string{"backend"}}}}
	if hasFrontendRepo(m2) {
		t.Fatal("no frontend repo → false")
	}
}
