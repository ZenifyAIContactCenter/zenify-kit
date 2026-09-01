package reconcile

import (
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

func TestBuild(t *testing.T) {
	m := &manifest.Manifest{Org: "ZenifyAIContactCenter", Repos: []manifest.Repo{
		{Name: "be", URL: "git@github.com:ZenifyAIContactCenter/be.git", Path: "be", Base: "origin/staging"},
		{Name: "web", URL: "git@github.com:ZenifyAIContactCenter/web.git", Path: "web", Base: "origin/staging"},
		{Name: "hub", URL: "git@github.com:ZenifyAIContactCenter/hub.git", Path: "hub", Base: "origin/staging"},
		{Name: "chatting", URL: "git@github.com:ZenifyAIContactCenter/chatting.git", Path: "chatting", Base: "origin/staging"},
		{Name: "notif", URL: "git@github.com:ZenifyAIContactCenter/notif.git", Path: "notif", Base: "origin/staging"},
		{Name: "secret-repo", URL: "git@github.com:ZenifyAIContactCenter/secret-repo.git", Path: "secret-repo", Base: "origin/staging"},
	}}
	access := map[string]ghx.RemoteRepo{
		"be": {Name: "be"}, "web": {Name: "web"}, "hub": {Name: "hub"},
		"chatting": {Name: "chatting"}, "notif": {Name: "notif"},
		// secret-repo absent → no access
	}
	scans := map[string]gitx.RepoState{
		"be":       {Cloned: false},
		"web":      {Cloned: true, Dirty: true},
		"hub":      {Cloned: true, NormalizedRemote: "ZenifyAIContactCenter/OTHER"},
		"chatting": {Cloned: true, HasClaude: false, Layout: "none"},
		"notif":    {Cloned: true, HasClaude: true, Layout: "new", Branch: "namph/feat/x"},
	}
	got := Build(m, access, scans)
	want := map[string]State{
		"be": Clone, "web": SkipDirty, "hub": WrongRemote,
		"chatting": Wire, "notif": Adopt, "secret-repo": NoAccess,
	}
	if len(got) != len(m.Repos) {
		t.Fatalf("plans = %d, want %d", len(got), len(m.Repos))
	}
	for _, p := range got {
		if want[p.Name] != p.State {
			t.Errorf("%s: State = %q, want %q (reason=%q)", p.Name, p.State, want[p.Name], p.Reason)
		}
	}
	// order preserved
	if got[0].Name != "be" || got[5].Name != "secret-repo" {
		t.Errorf("order not preserved: %v", got)
	}
}

func TestBuildOK(t *testing.T) {
	m := &manifest.Manifest{Org: "X", Repos: []manifest.Repo{
		{Name: "be", URL: "git@github.com:X/be.git", Path: "be", Base: "origin/staging"},
	}}
	access := map[string]ghx.RemoteRepo{"be": {Name: "be"}}
	scans := map[string]gitx.RepoState{"be": {
		Cloned: true, HasClaude: true, Layout: "new",
		NormalizedRemote: "X/be", Branch: "staging",
	}}
	got := Build(m, access, scans)
	if got[0].State != OK {
		t.Errorf("State = %q, want OK", got[0].State)
	}
}
