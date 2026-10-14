package manifest

import (
	"os"
	"path/filepath"
	"strings"
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

func TestParseWithOverlay_FromBytes(t *testing.T) {
	base := []byte("org: acme\nrepos:\n  - name: a\n    path: repos/a\n")
	dir := t.TempDir()
	ov := filepath.Join(dir, "overlay.yaml")
	if err := os.WriteFile(ov, []byte("repos:\n  - name: a\n    path: elsewhere/a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ParseWithOverlay(base, "embedded", ov)
	if err != nil {
		t.Fatalf("ParseWithOverlay: %v", err)
	}
	if m.Repos[0].Path != "elsewhere/a" {
		t.Fatalf("overlay not applied: %+v", m.Repos[0])
	}
	if _, err := Parse([]byte("repos: []\n"), "embedded"); err == nil {
		t.Fatal("empty org must error")
	}
}

func TestParseRepoSecretKeys(t *testing.T) {
	m, err := Parse([]byte(`org: o
secretKeys: [MONGO_URL]
repos:
  - name: a
    url: git@github.com:o/a.git
    path: repos/a
    base: origin/staging
    secretKeys: [A_URL_STG, A_URL_PRD]
  - name: b
    url: git@github.com:o/b.git
    path: repos/b
    base: origin/staging
`), "test")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := m.ByName("a")
	b, _ := m.ByName("b")
	if len(a.SecretKeys) != 2 || a.SecretKeys[0].Name != "A_URL_STG" || !a.SecretKeys[0].Secret || len(b.SecretKeys) != 0 {
		t.Fatalf("a=%v b=%v", a.SecretKeys, b.SecretKeys)
	}
}

// SC-7 at the source: lumi keys live on the lumi entry only, never in the global list.
func TestShippedManifestLumiKeysAreRepoScoped(t *testing.T) {
	m, err := Load(filepath.Join("..", "..", "manifest", "repos.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"LUMI_MYSQL_URL_STG", "LUMI_MYSQL_URL_PRD", "QDRANT_URL_STG", "QDRANT_URL_PRD", "LUMI_APP_URL_STG", "OPENROUTER_API_KEY"}
	lumi, ok := m.ByName("lumi-agent")
	if !ok || len(lumi.SecretKeys) != len(want) {
		t.Fatalf("lumi-agent secretKeys = %v, want %v", lumi.SecretKeys, want)
	}
	for i := range want {
		if lumi.SecretKeys[i].Name != want[i] {
			t.Fatalf("lumi-agent secretKeys = %v, want %v", lumi.SecretKeys, want)
		}
	}
	for _, k := range m.SecretKeys {
		if strings.HasPrefix(k.Name, "LUMI_") || strings.HasPrefix(k.Name, "QDRANT_") {
			t.Errorf("global secretKeys must not carry repo-scoped key %s", k.Name)
		}
	}
	for _, r := range m.Repos {
		if r.Name != "lumi-agent" && len(r.SecretKeys) > 0 {
			t.Errorf("unexpected repo keys on %s: %v", r.Name, r.SecretKeys)
		}
	}
}

func TestSecretKey_ObjectForm(t *testing.T) {
	m, err := Parse([]byte(`org: o
secretKeys:
  - name: MONGO_URL
    description: Mongo connection string
    howTo: Ask the DevOps lead
    scope: shared
    secret: true
  - name: E2E_DOMAIN
    description: d
    howTo: h
    scope: personal
    secret: false
repos:
  - name: a
    url: u
    path: p
    base: b
`), "test")
	if err != nil {
		t.Fatal(err)
	}
	want := SecretKey{Name: "MONGO_URL", Description: "Mongo connection string", HowTo: "Ask the DevOps lead", Scope: "shared", Secret: true}
	if m.SecretKeys[0] != want {
		t.Fatalf("got %+v, want %+v", m.SecretKeys[0], want)
	}
	if m.SecretKeys[1].Secret || m.SecretKeys[1].Scope != "personal" {
		t.Fatalf("explicit secret:false / personal lost: %+v", m.SecretKeys[1])
	}
}

func TestSecretKey_ObjectFormDefaultsSecretTrue(t *testing.T) {
	m, err := Parse([]byte("org: o\nsecretKeys:\n  - name: X\nrepos:\n  - {name: a, url: u, path: p, base: b}\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if !m.SecretKeys[0].Secret {
		t.Fatalf("omitted secret must default to true: %+v", m.SecretKeys[0])
	}
}

func TestSecretKey_StringForm(t *testing.T) {
	m, err := Parse([]byte("org: o\nsecretKeys:\n  - MONGO_URL\nrepos:\n  - {name: a, url: u, path: p, base: b}\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.SecretKeys) != 1 || m.SecretKeys[0] != (SecretKey{Name: "MONGO_URL", Secret: true}) {
		t.Fatalf("got %+v", m.SecretKeys)
	}
}

func TestAllKeys_GlobalThenRepoDedupedInOrder(t *testing.T) {
	m, err := Parse([]byte(`org: o
secretKeys: [A, B]
repos:
  - name: r1
    url: u
    path: p
    base: b
    secretKeys: [B, C]
  - name: r2
    url: u
    path: p2
    base: b
    secretKeys: [C, D]
`), "test")
	if err != nil {
		t.Fatal(err)
	}
	got := KeyNames(m.AllKeys())
	want := []string{"A", "B", "C", "D"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEmbeddedManifest_EveryKeyDescribed(t *testing.T) {
	m, err := Load(filepath.Join("..", "..", "manifest", "repos.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	keys := m.AllKeys()
	if len(keys) != 16 {
		t.Fatalf("want 16 keys, got %d", len(keys))
	}
	notSecret := map[string]bool{"E2E_DOMAIN": true, "MYSQL_HOST": true, "MYSQL_PORT": true, "MYSQL_DATABASE": true, "LUMI_APP_URL_STG": true}
	personal := map[string]bool{"E2E_EMAIL": true, "E2E_PASSWORD": true, "POSTHOG_API_KEY": true, "OPENROUTER_API_KEY": true}
	for _, k := range keys {
		if k.Description == "" {
			t.Errorf("%s: description must be non-empty", k.Name)
		}
		wantScope := "shared"
		if personal[k.Name] {
			wantScope = "personal"
		}
		if k.Scope != wantScope {
			t.Errorf("%s: scope %q, want %q", k.Name, k.Scope, wantScope)
		}
		if k.Secret == notSecret[k.Name] {
			t.Errorf("%s: secret=%v unexpected", k.Name, k.Secret)
		}
	}
}
