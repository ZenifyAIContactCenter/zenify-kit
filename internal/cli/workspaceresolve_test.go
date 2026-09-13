package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func mkMarker(t *testing.T, ws string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "manifest.json"), []byte(`{"entries":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveWorkspace_Table(t *testing.T) {
	home := t.TempDir()
	userHome := func() (string, error) { return home, nil }
	noEnv := func(string) string { return "" }

	wsA := filepath.Join(t.TempDir(), "A")
	mkMarker(t, wsA)
	nested := filepath.Join(wsA, "repos", "be", "src")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	plain := t.TempDir() // no marker, not under a workspace

	// pointer → wsA
	if err := writeWorkspacePointer(noEnv, userHome, wsA); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".zenify", "workspace"))
	if string(b) != wsA+"\n" {
		t.Fatalf("pointer content %q", b)
	}

	cases := []struct {
		name       string
		cwd, flag  string
		getenv     func(string) string
		wantPath   string
		wantSource wsSource
		wantOK     bool
	}{
		{"flag wins", plain, wsA, noEnv, wsA, wsFromFlag, true},
		{"flag dot means unset → marker", nested, ".", noEnv, wsA, wsFromMarker, true},
		{"marker at cwd", wsA, "", noEnv, wsA, wsFromMarker, true},
		{"marker at ancestor", nested, "", noEnv, wsA, wsFromMarker, true},
		{"pointer when no marker", plain, "", noEnv, wsA, wsFromPointer, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, src, ok := resolveWorkspace(c.cwd, c.flag, c.getenv, userHome)
			if ok != c.wantOK || src != c.wantSource || p != c.wantPath {
				t.Fatalf("got (%q,%q,%v) want (%q,%q,%v)", p, src, ok, c.wantPath, c.wantSource, c.wantOK)
			}
		})
	}

	// pointer to a dir WITHOUT marker → not ok
	stale := t.TempDir()
	if err := writeWorkspacePointer(noEnv, userHome, stale); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := resolveWorkspace(plain, "", noEnv, userHome); ok {
		t.Fatal("stale pointer must not resolve")
	}

	// $ZENIFY_HOME relocates the pointer
	zh := t.TempDir()
	env := func(k string) string {
		if k == "ZENIFY_HOME" {
			return zh
		}
		return ""
	}
	if err := writeWorkspacePointer(env, userHome, wsA); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(zh, "workspace")); err != nil {
		t.Fatalf("pointer not under ZENIFY_HOME: %v", err)
	}
	if p, src, ok := resolveWorkspace(plain, "", env, userHome); !ok || src != wsFromPointer || p != wsA {
		t.Fatalf("ZENIFY_HOME pointer: (%q,%q,%v)", p, src, ok)
	}

	// nothing at all → not ok
	if _, _, ok := resolveWorkspace(plain, "", noEnv, func() (string, error) { return t.TempDir(), nil }); ok {
		t.Fatal("no flag/marker/pointer must be !ok")
	}
}
