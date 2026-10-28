package release

import (
	"testing"
)

func TestBuildParticipationAndFlags(t *testing.T) {
	be := fakeRunner{out: map[string]string{
		"tag --list": "v261028.0\nv261104.0\n",
		"log --format=%h\x1f%s\x1f%an\x1f%b\x1e v261028.0..v261104.0":                      "5ed\x1ffix: a\x1fnamph\x1f\x1e" + "aaa\x1fMerge pull request #1 from o/hungnk/hotfix/x\x1fhungnk\x1f\x1e",
		"log --first-parent --format=%h\x1f%s\x1f%an\x1f%b\x1e v261028.0..v261104.0":       "5ed\x1ffix: a\x1fnamph\x1f\x1e" + "aaa\x1fMerge pull request #1 from o/hungnk/hotfix/x\x1fhungnk\x1f\x1e",
		"diff --name-only v261028.0..v261104.0":                                            "db/migrations/1.js\napp/models/chat_message.js\nfoo_test.go\n",
		"log --format=%h\x1f%s\x1f%an\x1f%b\x1e v261028.0..v261104.0 --not origin/staging": "9dc\x1ftemporary disable report api\x1fnamph\x1f\x1e",
	}}
	notif := fakeRunner{out: map[string]string{"tag --list": "v261028.0\n"}}
	router := dirRouter{per: map[string]fakeRunner{"/ws/be": be, "/ws/notif": notif}}
	loadPatterns := func(dir string) []string { return []string{"**/chat_*"} }
	resolve := func(name string) (string, bool) { return "/ws/" + name, true }

	rep := Build(router, resolve, []string{"be", "notif"}, "v261104.0", loadPatterns, func(string) []SpecMeta { return nil })

	if len(rep.Repos) != 2 || rep.Repos[0].Name != "be" || rep.Repos[1].Err != "chưa có tag v261104.0" { //znf:allow-lang
		t.Fatalf("repos=%+v", rep.Repos)
	}
	be0 := rep.Repos[0]
	if !be0.HasMigration || !be0.HasTestTouch || len(be0.SharedHits) == 0 {
		t.Errorf("flags: %+v", be0)
	}
	if len(be0.Regression) != 1 || be0.Regression[0].SHA != "9dc" {
		t.Errorf("regression: %+v", be0.Regression)
	}
	if len(be0.Hotfixes) != 1 || be0.PrevTag != "v261028.0" {
		t.Errorf("hotfix/prev: %+v", be0)
	}
	if len(be0.Changes) == 0 {
		t.Errorf("must have Changes after aggregate")
	}
	if len(rep.ShippingRepos) != 1 || rep.ShippingRepos[0] != "be" {
		t.Errorf("shipping: %v", rep.ShippingRepos)
	}
	// does the hotfix change 'x' have commit '9dc'? — notStaging marks by SHA; here the hotfix merge 'aaa'
	if rep.TotalHotfix < 1 {
		t.Errorf("TotalHotfix: %d", rep.TotalHotfix)
	}
}

func TestBuildFailOpenPerRepo(t *testing.T) {
	bad := fakeRunner{err: map[string]string{"tag --list": "boom"}}
	router := dirRouter{per: map[string]fakeRunner{"/ws/bad": bad}}
	resolve := func(name string) (string, bool) { return "/ws/" + name, true }
	rep := Build(router, resolve, []string{"bad"}, "v261104.0", func(string) []string { return nil }, func(string) []SpecMeta { return nil })
	if len(rep.Repos) != 1 || rep.Repos[0].Err == "" {
		t.Errorf("expected fail-open note, got %+v", rep.Repos)
	}
}

func TestBuildUsesResolverNotFlatJoin(t *testing.T) {
	// resolve returns an arbitrary path (simulating nesting); Build must list tags on that path.
	seen := map[string]bool{}
	resolve := func(name string) (string, bool) {
		return "/ws/repos/" + name, true
	}
	r := fakeRunner{calls: seen}
	_ = Build(r, resolve, []string{"svc-a"}, "v261104.0", func(dir string) []string { return nil }, func(string) []SpecMeta { return nil })
	if !seen["/ws/repos/svc-a"] {
		t.Fatalf("Build must use the resolver's path, dirs called: %+v", seen)
	}
}

func TestBuildResolveMiss(t *testing.T) {
	resolve := func(name string) (string, bool) { return "", false }
	rep := Build(fakeRunner{}, resolve, []string{"ghost"}, "v261104.0", func(string) []string { return nil }, func(string) []SpecMeta { return nil })
	if len(rep.Repos) != 1 || rep.Repos[0].Err == "" {
		t.Errorf("expected resolve-miss fail-open note, got %+v", rep.Repos)
	}
}

func TestIsReleaseTag(t *testing.T) {
	for tag, want := range map[string]bool{
		"v261104.0": true, "v261104.12": true,
		"v0.52.0": false, "release84": false, "v261104.0-rc": false, "-v261104.0": false,
	} {
		if got := IsReleaseTag(tag); got != want {
			t.Errorf("IsReleaseTag(%q) = %v, want %v", tag, got, want)
		}
	}
}
