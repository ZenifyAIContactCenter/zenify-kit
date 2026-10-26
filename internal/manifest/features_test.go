package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSetFeature_PreservesReposAndComments(t *testing.T) {
	p := writeTmp(t, "ov.yaml", "# mine\nrepos:\n  - name: a\n    path: elsewhere/a\n")
	if err := SetFeature(p, "github.issues", true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "# mine") || !strings.Contains(string(b), "github.issues: true") {
		t.Fatalf("overlay lost comment or feature:\n%s", b)
	}
	got, err := ReadFeatures(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]bool{"github.issues": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	m, err := ParseWithOverlay([]byte("org: acme\nrepos:\n  - name: a\n    path: repos/a\n"), "embedded", p)
	if err != nil {
		t.Fatal(err)
	}
	if m.Repos[0].Path != "elsewhere/a" {
		t.Fatalf("path = %q", m.Repos[0].Path)
	}
}

func TestSetFeature_CreatesFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "new.yaml")
	if err := SetFeature(p, "github.issues", true); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFeatures(p)
	if err != nil || !got["github.issues"] {
		t.Fatalf("got %v err %v", got, err)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
}

func TestSetFeature_UnknownKey(t *testing.T) {
	const orig = "repos:\n  - name: a\n"
	p := writeTmp(t, "ov.yaml", orig)
	err := SetFeature(p, "bogus", true)
	if err == nil || !strings.Contains(err.Error(), `unknown feature "bogus"`) {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != orig {
		t.Fatalf("file changed: %s", b)
	}
}

func TestSetFeature_BrokenYAMLNotOverwritten(t *testing.T) {
	const orig = "repos: ["
	p := writeTmp(t, "ov.yaml", orig)
	if err := SetFeature(p, "github.issues", true); err == nil {
		t.Fatal("expected error")
	}
	if b, _ := os.ReadFile(p); string(b) != orig {
		t.Fatalf("file overwritten: %s", b)
	}
	if _, err := ReadFeatures(p); err == nil || !strings.Contains(err.Error(), "parse overlay") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadFeatures_Missing(t *testing.T) {
	got, err := ReadFeatures(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestReadFeatures_IgnoresUnknownKeys(t *testing.T) {
	p := writeTmp(t, "ov.yaml", "features:\n  github.issues: true\n  other: true\n")
	got, err := ReadFeatures(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]bool{"github.issues": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestSetFeature_Off(t *testing.T) {
	p := writeTmp(t, "ov.yaml", "features:\n  github.issues: true\n")
	if err := SetFeature(p, "github.issues", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "github.issues: false") {
		t.Fatalf("got:\n%s", b)
	}
}
