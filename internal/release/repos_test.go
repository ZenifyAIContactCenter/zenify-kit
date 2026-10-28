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
	got, err := Resolve(nil, "/ws", readFile, nil)
	if err != nil || len(got) != 2 || got[0] != "contact-center-be" || got[1] != "chatting" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestResolveConfigSkipsComments(t *testing.T) {
	readFile := func(p string) ([]byte, error) {
		return []byte("# header\ncontact-center-be\n  # indented comment\nchatting\n"), nil
	}
	got, err := Resolve(nil, "/ws", readFile, nil)
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
		"/ws/repoA": {out: map[string]string{"tag --list": "v261028.0\n"}},
		"/ws/repoB": {out: map[string]string{"tag --list": "v0.52.0\nrelease84\n"}},
	}}
	got, err := Resolve(fr, "/ws", readFile, discovered)
	if err != nil || len(got) != 1 || got[0] != "repoA" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func pathReader(files map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if s, ok := files[p]; ok {
			return []byte(s), nil
		}
		return nil, errors.New("no file")
	}
}

func TestPinList_ReadsZenifyDir(t *testing.T) {
	got, ok := pinList("/ws", pathReader(map[string]string{"/ws/.zenify/release-repos.txt": "chatting\n"}))
	if !ok || len(got) != 1 || got[0] != "chatting" {
		t.Fatalf("got=%v ok=%v", got, ok)
	}
}

func TestPinList_LegacyZnfStillRead(t *testing.T) {
	got, ok := pinList("/ws", pathReader(map[string]string{"/ws/.znf/release-repos.txt": "chatting\n"}))
	if !ok || len(got) != 1 || got[0] != "chatting" {
		t.Fatalf("legacy .znf must still be read: got=%v ok=%v", got, ok)
	}
}

func TestPinList_ZenifyWinsOverZnf(t *testing.T) {
	got, ok := pinList("/ws", pathReader(map[string]string{
		"/ws/.zenify/release-repos.txt": "new-repo\n",
		"/ws/.znf/release-repos.txt":    "old-repo\n",
	}))
	if !ok || len(got) != 1 || got[0] != "new-repo" {
		t.Fatalf(".zenify must win: got=%v ok=%v", got, ok)
	}
}
