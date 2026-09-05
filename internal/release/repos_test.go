package release

import (
	"errors"
	"testing"
)

// dirRouter: fake Runner trả output khác nhau theo dir. Dùng chung repos_test/release_test.
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

func TestResolveAutoDetect(t *testing.T) {
	readFile := func(p string) ([]byte, error) { return nil, errors.New("no file") }
	readDir := func(p string) ([]string, error) { return []string{"repoA", "repoB"}, nil }
	fr := dirRouter{per: map[string]fakeRunner{
		"/ws/repoA": {out: map[string]string{"branch -r": "  origin/release84\n"}},
		"/ws/repoB": {out: map[string]string{"branch -r": "  origin/release83\n"}},
	}}
	got, err := Resolve(fr, "/ws", 84, readFile, readDir)
	if err != nil || len(got) != 1 || got[0] != "repoA" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
