package release

import "testing"

func TestBuildParticipationAndFlags(t *testing.T) {
	be := fakeRunner{out: map[string]string{
		"branch -r": "  origin/release83\n  origin/release84\n  origin/staging\n",
		"log --format=%h\x1f%s origin/release83..origin/release84":                      "5ed\x1ffix: a\naaa\x1fMerge pull request #1 from o/hungnk/hotfix/x\n",
		"diff --name-only origin/release83..origin/release84":                           "db/migrations/1.js\napp/models/chat_message.js\nfoo_test.go\n",
		"log --format=%h\x1f%s origin/release83..origin/release84 --not origin/staging": "9dc\x1ftemporary disable report api\n",
		"merge-base origin/release84 origin/staging":                                    "base1\n",
		"log -1 --format=%ci base1":                                                     "2026-08-26 17:55:55 +0700\n",
	}}
	notif := fakeRunner{out: map[string]string{"branch -r": "  origin/release83\n  origin/staging\n"}}
	router := dirRouter{per: map[string]fakeRunner{"/ws/be": be, "/ws/notif": notif}}
	loadPatterns := func(dir string) []string { return []string{"**/chat_*"} }

	rep := Build(router, "/ws", []string{"be", "notif"}, 84, loadPatterns)

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
}

func TestBuildFailOpenPerRepo(t *testing.T) {
	bad := fakeRunner{err: map[string]string{"branch -r": "boom"}}
	router := dirRouter{per: map[string]fakeRunner{"/ws/bad": bad}}
	rep := Build(router, "/ws", []string{"bad"}, 84, func(string) []string { return nil })
	if len(rep.Repos) != 1 || rep.Repos[0].Err == "" {
		t.Errorf("expected fail-open note, got %+v", rep.Repos)
	}
}
