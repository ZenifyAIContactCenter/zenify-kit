package release

import (
	"strings"
	"testing"
)

func TestBuildParticipationAndFlags(t *testing.T) {
	be := fakeRunner{out: map[string]string{
		"branch -r": "  origin/release83\n  origin/release84\n  origin/staging\n",
		"log --format=%h\x1f%s\x1f%an\x1f%b\x1e origin/release83..origin/release84":                      "5ed\x1ffix: a\x1fnamph\x1f\x1e" + "aaa\x1fMerge pull request #1 from o/hungnk/hotfix/x\x1fhungnk\x1f\x1e",
		"log --first-parent --format=%h\x1f%s\x1f%an\x1f%b\x1e origin/release83..origin/release84":       "5ed\x1ffix: a\x1fnamph\x1f\x1e" + "aaa\x1fMerge pull request #1 from o/hungnk/hotfix/x\x1fhungnk\x1f\x1e",
		"diff --name-only origin/release83..origin/release84":                                          "db/migrations/1.js\napp/models/chat_message.js\nfoo_test.go\n",
		"log --format=%h\x1f%s\x1f%an\x1f%b\x1e origin/release83..origin/release84 --not origin/staging": "9dc\x1ftemporary disable report api\x1fnamph\x1f\x1e",
		"merge-base origin/release84 origin/staging":                                              "base1\n",
		"log -1 --format=%ci base1":                                                               "2026-08-26 17:55:55 +0700\n",
	}}
	notif := fakeRunner{out: map[string]string{"branch -r": "  origin/release83\n  origin/staging\n"}}
	router := dirRouter{per: map[string]fakeRunner{"/ws/be": be, "/ws/notif": notif}}
	loadPatterns := func(dir string) []string { return []string{"**/chat_*"} }
	resolve := func(name string) (string, bool) { return "/ws/" + name, true }

	rep := Build(router, resolve, []string{"be", "notif"}, 84, loadPatterns, func(string) []SpecMeta { return nil })

	if len(rep.Repos) != 1 || rep.Repos[0].Name != "be" {
		t.Fatalf("participating=%v", rep.Repos)
	}
	be0 := rep.Repos[0]
	if !be0.HasMigration || !be0.HasTestTouch || len(be0.SharedHits) == 0 {
		t.Errorf("flags: %+v", be0)
	}
	if len(be0.Regression) != 1 || be0.Regression[0].SHA != "9dc" {
		t.Errorf("regression: %+v", be0.Regression)
	}
	if len(be0.Hotfixes) != 1 || be0.CutDate != "2026-08-26" {
		t.Errorf("hotfix/cut: %+v", be0)
	}
	if len(rep.NotShipped) != 1 || rep.NotShipped[0] != "notif" {
		t.Errorf("notshipped: %v", rep.NotShipped)
	}
	if len(be0.Changes) == 0 {
		t.Errorf("phải có Changes sau aggregate")
	}
	if len(rep.ShippingRepos) != 1 || rep.ShippingRepos[0] != "be" {
		t.Errorf("shipping: %v", rep.ShippingRepos)
	}
	// hotfix change 'x' có commit '9dc'? — notStaging đánh dấu qua SHA; ở đây hotfix merge 'aaa'
	if rep.TotalHotfix < 1 {
		t.Errorf("TotalHotfix: %d", rep.TotalHotfix)
	}
}

func TestBuildFailOpenPerRepo(t *testing.T) {
	bad := fakeRunner{err: map[string]string{"branch -r": "boom"}}
	router := dirRouter{per: map[string]fakeRunner{"/ws/bad": bad}}
	resolve := func(name string) (string, bool) { return "/ws/" + name, true }
	rep := Build(router, resolve, []string{"bad"}, 84, func(string) []string { return nil }, func(string) []SpecMeta { return nil })
	if len(rep.Repos) != 1 || rep.Repos[0].Err == "" {
		t.Errorf("expected fail-open note, got %+v", rep.Repos)
	}
}

func TestBuildUsesResolverNotFlatJoin(t *testing.T) {
	// resolve trả path tùy ý (giả nested); Build phải gọi ReleaseNums trên path đó.
	seen := map[string]bool{}
	resolve := func(name string) (string, bool) {
		return "/ws/repos/" + name, true
	}
	r := fakeRunner{calls: seen}
	_ = Build(r, resolve, []string{"svc-a"}, 84, func(dir string) []string { return nil }, func(string) []SpecMeta { return nil })
	if !seen["/ws/repos/svc-a"] {
		t.Fatalf("Build phải dùng path từ resolver, các dir đã gọi: %+v", seen)
	}
}

func TestBuildUnreleasedRangeStagingDeterministic(t *testing.T) {
	// release88 là cao nhất; range incremental = origin/release88..origin/staging.
	fr := fakeRunner{out: map[string]string{
		"branch -r": "  origin/release87\n  origin/release88\n  origin/staging\n",
		"log --format=%h\x1f%s\x1f%an\x1f%b\x1e origin/release88..origin/staging": "h1\x1ffeat(alpha): a\x1fnamph\x1f\x1e",
	}}
	resolve := func(name string) (string, bool) { return "/ws/" + name, true }
	noPatterns := func(string) []string { return nil }
	noSpecs := func(string) []SpecMeta { return nil }

	rep := BuildUnreleased(fr, resolve, []string{"be"}, 88, noPatterns, noSpecs)
	if !rep.Unreleased {
		t.Fatalf("Report.Unreleased phải true")
	}
	if rep.N != 88 {
		t.Fatalf("N phải là latest cut (88) để render 'sau R88': %d", rep.N)
	}
	// deterministic: chạy hai lần cùng state → render giống hệt.
	a := Render(rep, false)
	b := Render(BuildUnreleased(fr, resolve, []string{"be"}, 88, noPatterns, noSpecs), false)
	if a != b {
		t.Errorf("render incremental phải deterministic")
	}
	if !strings.Contains(a, "hình thành") {
		t.Errorf("header unreleased phải khác '# Release N': %s", a)
	}
}

func TestBuildResolveMiss(t *testing.T) {
	resolve := func(name string) (string, bool) { return "", false }
	rep := Build(fakeRunner{}, resolve, []string{"ghost"}, 84, func(string) []string { return nil }, func(string) []SpecMeta { return nil })
	if len(rep.Repos) != 1 || rep.Repos[0].Err == "" {
		t.Errorf("expected resolve-miss fail-open note, got %+v", rep.Repos)
	}
}
