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
	got := Build(m, access, scans, nil)
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
	got := Build(m, access, scans, nil)
	if got[0].State != OK {
		t.Errorf("State = %q, want OK", got[0].State)
	}
}

func TestBuild_DirtyReasonIsActionable(t *testing.T) {
	m := &manifest.Manifest{Org: "X", Repos: []manifest.Repo{{Name: "be", URL: "git@github.com:X/be.git", Path: "repos/be", Base: "origin/staging"}}}
	got := Build(m, map[string]ghx.RemoteRepo{"be": {Name: "be"}}, map[string]gitx.RepoState{"be": {Cloned: true, Dirty: true}}, nil)
	want := "uncommitted changes — commit or stash, then re-run (kit only writes .claude/settings.local.json and .git/info/exclude)"
	if got[0].State != SkipDirty || got[0].Reason != want {
		t.Fatalf("got %q / %q", got[0].State, got[0].Reason)
	}
}

func TestBuild_RelocateFromSources(t *testing.T) {
	m := &manifest.Manifest{Org: "X", Repos: []manifest.Repo{
		{Name: "be", URL: "git@github.com:X/be.git", Path: "repos/be", Base: "origin/staging"},
		{Name: "web", URL: "git@github.com:X/web.git", Path: "repos/web", Base: "origin/staging"},
		{Name: "hub", URL: "git@github.com:X/hub.git", Path: "repos/hub", Base: "origin/staging"},
		{Name: "old", URL: "git@github.com:X/old.git", Path: "repos/old", Base: "origin/staging"},
	}}
	access := map[string]ghx.RemoteRepo{"be": {Name: "be"}, "web": {Name: "web"}, "hub": {Name: "hub"}, "old": {Name: "old"}}
	scans := map[string]gitx.RepoState{
		"be":  {Cloned: false},
		"web": {Cloned: false},
		"hub": {Cloned: true, HasClaude: true, Layout: "new", NormalizedRemote: "X/hub", Branch: "staging"},
		"old": {Cloned: true, Layout: "old", NormalizedRemote: "X/old"},
	}
	sources := map[string]Source{
		"be":  {Path: "/home/u/projects/be"},                      // dirty or not — still relocates
		"web": {Path: "/home/u/projects/web", HasWorktrees: true}, // blocker
		"hub": {Path: "/home/u/projects/hub"},                     // dest already cloned → blocker
	}
	got := Build(m, access, scans, sources)
	byName := map[string]RepoPlan{}
	for _, p := range got {
		byName[p.Name] = p
	}
	if p := byName["be"]; p.State != Relocate || p.From != "/home/u/projects/be" ||
		p.Reason != "found at /home/u/projects/be — will move to repos/be, old path stays as a link" {
		t.Fatalf("be: %+v", p)
	}
	if p := byName["web"]; p.State != Skip || p.Reason != "has attached worktrees — remove them or move by hand" {
		t.Fatalf("web: %+v", p)
	}
	if p := byName["hub"]; p.State != Skip || p.Reason != "repos/hub already exists (also found at /home/u/projects/hub)" {
		t.Fatalf("hub: %+v", p)
	}
	if p := byName["old"]; p.State != Migrate { // existing meaning untouched
		t.Fatalf("old: %+v", p)
	}
}

func TestBuild_RelocateWinsOverDirty(t *testing.T) {
	m := &manifest.Manifest{Org: "X", Repos: []manifest.Repo{{Name: "be", URL: "git@github.com:X/be.git", Path: "repos/be", Base: "origin/staging"}}}
	// scans is keyed by manifest path (absent there); dirty state of the SOURCE is not consulted
	got := Build(m, map[string]ghx.RemoteRepo{"be": {Name: "be"}}, map[string]gitx.RepoState{"be": {Cloned: false}}, map[string]Source{"be": {Path: "/x/be"}})
	if got[0].State != Relocate {
		t.Fatalf("got %+v", got[0])
	}
}
