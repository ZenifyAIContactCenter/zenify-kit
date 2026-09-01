package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTmp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad(t *testing.T) {
	p := writeTmp(t, "repos.yaml", `org: ZenifyAIContactCenter
repos:
  - name: contact-center-be
    url: git@github.com:ZenifyAIContactCenter/contact-center-be.git
    path: contact-center-be
    base: origin/staging
    tags: [primary, backend]
  - name: chatting
    url: git@github.com:ZenifyAIContactCenter/chatting.git
    path: chatting
    base: origin/staging
    tags: [realtime]
`)
	m, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Org != "ZenifyAIContactCenter" {
		t.Errorf("Org = %q", m.Org)
	}
	if len(m.Repos) != 2 {
		t.Fatalf("Repos = %d, want 2", len(m.Repos))
	}
	be, ok := m.ByName("contact-center-be")
	if !ok || be.Base != "origin/staging" || len(be.Tags) != 2 {
		t.Errorf("ByName be = %+v ok=%v", be, ok)
	}
	if _, ok := m.ByName("nope"); ok {
		t.Errorf("ByName nope should be false")
	}
}

func TestLoadRejectsEmpty(t *testing.T) {
	p := writeTmp(t, "repos.yaml", "org: X\nrepos: []\n")
	if _, err := Load(p); err == nil {
		t.Error("expected error for empty repos")
	}
	p2 := writeTmp(t, "r2.yaml", "repos:\n  - name: a\n    url: u\n    path: a\n    base: b\n")
	if _, err := Load(p2); err == nil {
		t.Error("expected error for missing org")
	}
}

func TestLoadWithOverlay(t *testing.T) {
	base := writeTmp(t, "repos.yaml", `org: ZenifyAIContactCenter
repos:
  - name: contact-center-be
    url: u
    path: contact-center-be
    base: origin/staging
`)
	overlay := writeTmp(t, "overlay.yaml", `repos:
  - name: contact-center-be
    path: /Users/me/custom/ccbe
`)
	m, err := LoadWithOverlay(base, overlay)
	if err != nil {
		t.Fatalf("LoadWithOverlay: %v", err)
	}
	be, _ := m.ByName("contact-center-be")
	if be.Path != "/Users/me/custom/ccbe" {
		t.Errorf("overlay Path = %q", be.Path)
	}
	if be.Base != "origin/staging" {
		t.Errorf("overlay clobbered Base = %q", be.Base)
	}
	// missing overlay file is not an error
	if _, err := LoadWithOverlay(base, filepath.Join(t.TempDir(), "none.yaml")); err != nil {
		t.Errorf("missing overlay should not error: %v", err)
	}
}
