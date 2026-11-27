package infra

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Business rule (FR-2.1): the base env's hosts carry no slug (web.zenify.test);
// any other env keeps -<slug>, so two envs never share a host.
func TestHost(t *testing.T) {
	if got := Host("web", BaseSlug, "zenify.test"); got != "web.zenify.test" {
		t.Fatalf("base host = %q", got)
	}
	if got := Host("web", "feat-x", "zenify.test"); got != "web-feat-x.zenify.test" {
		t.Fatalf("slug host = %q", got)
	}
}

func TestLoadStateMissing(t *testing.T) {
	s, err := LoadState(t.TempDir())
	if s != nil || err != nil {
		t.Fatalf("got %v, %v; want nil, nil", s, err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	home := t.TempDir()
	want := &State{Domain: "a.test", K3sVersion: "v1", ArgoCDVersion: "v2", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	if err := SaveState(home, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(home)
	if err != nil || got == nil || *got != *want {
		t.Fatalf("got %+v, %v; want %+v", got, err, want)
	}
	fi, err := os.Stat(filepath.Join(home, ".zenify", "infra", "state.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v err %v", fi, err)
	}
	if err := RemoveState(home); err != nil {
		t.Fatal(err)
	}
	if s, err := LoadState(home); s != nil || err != nil {
		t.Fatalf("after remove: %v, %v", s, err)
	}
}

func TestLoadStateCorrupt(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".zenify", "infra")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(home); err == nil {
		t.Fatal("corrupt state must error")
	}
}
