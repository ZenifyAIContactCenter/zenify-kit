package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

func envStepFixture(t *testing.T) (string, []byte, *manifest.Manifest, *managed.Manifest) {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".zenify", ".env")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	orig := []byte("A='a1'\nB='secretval'\n")
	if err := os.WriteFile(p, orig, 0o600); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{SecretKeys: []manifest.SecretKey{{Name: "A", Secret: true}}}
	return p, orig, m, &managed.Manifest{EnvKeys: []string{"A", "B"}}
}

func TestRenderEnvStep_DryRunWritesNothing(t *testing.T) {
	p, orig, m, owned := envStepFixture(t)
	var out bytes.Buffer
	if err := renderEnvStep(&out, m, p, owned, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "xoá B") { //znf:allow-lang
		t.Fatalf("output lacks removal of B: %q", out.String())
	}
	if strings.Contains(out.String(), "secretval") {
		t.Fatalf("output leaks a value: %q", out.String())
	}
	got, _ := os.ReadFile(p)
	if !bytes.Equal(got, orig) {
		t.Fatalf("dry-run changed the file: %q", got)
	}
	if !reflect.DeepEqual(owned.EnvKeys, []string{"A", "B"}) {
		t.Fatalf("EnvKeys changed: %v", owned.EnvKeys)
	}
}

func TestRenderEnvStep_ApplyRecordsKeys(t *testing.T) {
	p, _, m, owned := envStepFixture(t)
	var out bytes.Buffer
	if err := renderEnvStep(&out, m, p, owned, false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if strings.Contains(string(got), "B=") {
		t.Fatalf("B should be removed: %q", got)
	}
	if !strings.Contains(string(got), "A='a1'") {
		t.Fatalf("A value lost: %q", got)
	}
	if !reflect.DeepEqual(owned.EnvKeys, []string{"A"}) {
		t.Fatalf("EnvKeys = %v, want [A]", owned.EnvKeys)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", st.Mode().Perm())
	}
}

func TestRenderEnvStep_PrintsShadowedNamesOnly(t *testing.T) {
	p, _, m, owned := envStepFixture(t)
	if err := os.WriteFile(p, []byte("A='fake-old'\nA='fake-new'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := renderEnvStep(&out, m, p, owned, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "env: giữ giá trị trùng của A dưới dạng comment") { //znf:allow-lang
		t.Fatalf("output lacks the shadowed line: %q", out.String())
	}
	if strings.Contains(out.String(), "fake-") {
		t.Fatalf("output leaks a value: %q", out.String())
	}
}
