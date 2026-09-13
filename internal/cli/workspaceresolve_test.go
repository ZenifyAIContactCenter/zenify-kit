package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func TestWorkspaceSettingsPath_FollowsResolvedWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", filepath.Join(home, ".zenify"))
	ws := filepath.Join(home, "Developer", "zenify")
	mkMarker(t, ws)
	if err := writeWorkspacePointer(os.Getenv, os.UserHomeDir, ws); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	t.Chdir(t.TempDir()) // cwd is NOT inside the workspace
	defer func() { _ = os.Chdir(wd) }()
	if got := workspaceSettingsPath(); got != filepath.Join(ws, ".claude", "settings.local.json") {
		t.Fatalf("settings path = %q", got)
	}
}

func TestSecretPresenceCheck_ReadsWorkspaceSettings(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(ws, ".claude", "settings.local.json")
	if err := os.WriteFile(settings, []byte(`{"env":{"MONGO_URL":"mongodb://x"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c := secretPresenceCheck(func(string) string { return "" }, func() string { return settings })
	ok, detail := c.Run()
	if !ok || !strings.Contains(detail, "MONGO_URL=present") {
		t.Fatalf("ok=%v detail=%q", ok, detail)
	}
}

func TestNoMaintainerPathInBinary(t *testing.T) {
	root := filepath.Join("..", "..", "internal")
	var hits []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, _ := os.ReadFile(p)
		for i, ln := range strings.Split(string(b), "\n") {
			if strings.Contains(ln, "WorkingSpace/zenify") && !strings.Contains(strings.TrimSpace(ln), "//") {
				hits = append(hits, p+":"+strconv.Itoa(i+1))
			}
		}
		return nil
	})
	if len(hits) > 0 {
		t.Fatalf("maintainer path still hardcoded: %v", hits)
	}
}
