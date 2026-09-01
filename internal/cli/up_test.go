package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
)

type fakeGH struct{ auth, list []byte }

func (f fakeGH) Run(args ...string) ([]byte, error) {
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
