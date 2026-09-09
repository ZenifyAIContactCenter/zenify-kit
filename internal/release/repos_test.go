package release

import (
	"errors"
	"testing"

	wspkg "github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

// dirRouter: a fake Runner that returns different output per dir. Shared by repos_test/release_test.
type dirRouter struct {
	base fakeRunner
	per  map[string]fakeRunner
}

func (d dirRouter) Run(dir string, args ...string) ([]byte, error) {
	if f, ok := d.per[dir]; ok {
		return f.Run(dir, args...)
	}
	return d.base.Run(dir, args...)
}

func TestResolveFromConfig(t *testing.T) {
	readFile := func(p string) ([]byte, error) {
		return []byte("contact-center-be\nchatting\n\n"), nil
	}
	got, err := Resolve(nil, "/ws", 84, readFile, nil)
	if err != nil || len(got) != 2 || got[0] != "contact-center-be" || got[1] != "chatting" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestResolveConfigSkipsComments(t *testing.T) {
	readFile := func(p string) ([]byte, error) {
		return []byte("# header\ncontact-center-be\n  # indented comment\nchatting\n"), nil
	}
	got, err := Resolve(nil, "/ws", 84, readFile, nil)
	if err != nil || len(got) != 2 || got[0] != "contact-center-be" || got[1] != "chatting" {
		t.Fatalf("comment lines must be dropped: got=%v err=%v", got, err)
	}
}

func TestResolveAutoDetect(t *testing.T) {
	readFile := func(p string) ([]byte, error) { return nil, errors.New("no file") }
	discovered := []wspkg.Repo{
		{Name: "repoA", Path: "/ws/repoA"},
		{Name: "repoB", Path: "/ws/repoB"},
	}
	fr := dirRouter{per: map[string]fakeRunner{
		"/ws/repoA": {out: map[string]string{"branch -r": "  origin/release84\n"}},
		"/ws/repoB": {out: map[string]string{"branch -r": "  origin/release83\n"}},
	}}
	got, err := Resolve(fr, "/ws", 84, readFile, discovered)
	if err != nil || len(got) != 1 || got[0] != "repoA" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

// ResolveUnreleased's auto-detect DIFFERS from Resolve: it takes EVERY repo with ≥1 release
// branch, not requiring release<n>. repoB (release83, no release84) STILL gets included — because
// unreleased is pending-deploy for every deployed repo. (With the same input Resolve(n=84) would
// return only repoA.)
func TestResolveUnreleasedIncludesAllReleaseRepos(t *testing.T) {
	readFile := func(p string) ([]byte, error) { return nil, errors.New("no file") }
	discovered := []wspkg.Repo{
		{Name: "repoA", Path: "/ws/repoA"},
		{Name: "repoB", Path: "/ws/repoB"},
		{Name: "repoC", Path: "/ws/repoC"},
	}
	fr := dirRouter{per: map[string]fakeRunner{
		"/ws/repoA": {out: map[string]string{"branch -r": "  origin/release84\n"}},
		"/ws/repoB": {out: map[string]string{"branch -r": "  origin/release83\n"}},
		"/ws/repoC": {out: map[string]string{"branch -r": "  origin/main\n"}}, // no release branch → excluded
	}}
	got, err := ResolveUnreleased(fr, "/ws", readFile, discovered)
	if err != nil || len(got) != 2 || got[0] != "repoA" || got[1] != "repoB" {
		t.Fatalf("must include both repoA+repoB (have a release), exclude repoC: got=%v err=%v", got, err)
	}
}

// ResolveUnreleased still honors the `.znf/release-repos.txt` pin when present.
func TestResolveUnreleasedHonorsPin(t *testing.T) {
	readFile := func(p string) ([]byte, error) { return []byte("chatting\n"), nil }
	got, err := ResolveUnreleased(nil, "/ws", readFile, nil)
	if err != nil || len(got) != 1 || got[0] != "chatting" {
		t.Fatalf("pin must be honored: got=%v err=%v", got, err)
	}
}
