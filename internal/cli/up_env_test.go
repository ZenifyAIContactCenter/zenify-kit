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
	return p, orig, m, &managed.Manifest{EnvKeys: []string{"A", "B"}, EnvKeysVersion: "dev"}
}

func TestRenderEnvStep_DryRunWritesNothing(t *testing.T) {
	p, orig, m, owned := envStepFixture(t)
	var out bytes.Buffer
	if err := renderEnvStep(&out, m, "embedded", p, owned, true); err != nil {
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
	if err := renderEnvStep(&out, m, "embedded", p, owned, false); err != nil {
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
	if err := renderEnvStep(&out, m, "embedded", p, owned, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "env: giữ giá trị trùng của A dưới dạng comment") { //znf:allow-lang
		t.Fatalf("output lacks the shadowed line: %q", out.String())
	}
	if strings.Contains(out.String(), "fake-") {
		t.Fatalf("output leaks a value: %q", out.String())
	}
}

func TestEnvDeletionGate(t *testing.T) {
	for _, c := range []struct {
		src, current, recorded string
		allowed                bool
		reason                 string
	}{
		{"embedded", "dev", "dev", true, ""},
		{"embedded", "v0.41.0", "v0.40.0", true, ""},
		{"embedded", "0.40.0", "v0.40.0", true, ""},
		{"embedded", "v0.39.0", "v0.40.0", false, "binary cũ hơn bản đã ghi"},                                                 //znf:allow-lang
		{"embedded", "dev", "v0.40.0", false, "binary cũ hơn bản đã ghi"},                                                     //znf:allow-lang
		{"embedded", "v0.40.0", "dev", false, "binary cũ hơn bản đã ghi"},                                                     //znf:allow-lang
		{"embedded", "v0.40.0", "", false, "binary cũ hơn bản đã ghi"},                                                        //znf:allow-lang
		{"embedded", "v0.40.0", "garbage", false, "binary cũ hơn bản đã ghi"},                                                 //znf:allow-lang
		{filepath.Join("manifest", "repos.yaml"), "dev", "dev", false, "manifest không phải bản nhúng (manifest/repos.yaml)"}, //znf:allow-lang
	} {
		ok, reason := envDeletionGate(c.src, c.current, c.recorded)
		if ok != c.allowed || reason != c.reason {
			t.Errorf("gate(%q,%q,%q) = %v %q, want %v %q", c.src, c.current, c.recorded, ok, reason, c.allowed, c.reason)
		}
	}
}

func TestRenderEnvStep_NonEmbeddedSourceKeepsKey(t *testing.T) {
	for _, dry := range []bool{true, false} {
		p, orig, m, owned := envStepFixture(t)
		var out bytes.Buffer
		if err := renderEnvStep(&out, m, "old/repos.yaml", p, owned, dry); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "env: giữ B — manifest không phải bản nhúng (old/repos.yaml)") || strings.Contains(out.String(), "xoá") { //znf:allow-lang
			t.Fatalf("dry=%v output = %q", dry, out.String())
		}
		if strings.Contains(out.String(), "secretval") {
			t.Fatalf("output leaks a value: %q", out.String())
		}
		got, _ := os.ReadFile(p)
		if dry {
			if !bytes.Equal(got, orig) {
				t.Fatalf("dry-run changed the file: %q", got)
			}
			continue
		}
		if !strings.Contains(string(got), "B='secretval'") {
			t.Fatalf("B value lost: %q", got)
		}
		if !reflect.DeepEqual(owned.EnvKeys, []string{"A", "B"}) {
			t.Fatalf("EnvKeys = %v, want [A B]", owned.EnvKeys)
		}
		// a later trustworthy run deletes it
		out.Reset()
		if err := renderEnvStep(&out, m, "embedded", p, owned, false); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(p); strings.Contains(string(got), "B=") || !strings.Contains(out.String(), "xoá B") { //znf:allow-lang
			t.Fatalf("B not deleted on the trusted run: %q / %q", got, out.String())
		}
	}
}

func TestRenderEnvStep_OlderBinaryKeepsKey(t *testing.T) {
	p, _, m, owned := envStepFixture(t)
	owned.EnvKeysVersion = "v99.0.0" // test binary is "dev": cannot prove it is not older
	var out bytes.Buffer
	if err := renderEnvStep(&out, m, "embedded", p, owned, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "env: giữ B — binary cũ hơn bản đã ghi") { //znf:allow-lang
		t.Fatalf("output = %q", out.String())
	}
	if got, _ := os.ReadFile(p); !strings.Contains(string(got), "B='secretval'") {
		t.Fatalf("B value lost: %q", got)
	}
	if !reflect.DeepEqual(owned.EnvKeys, []string{"A", "B"}) {
		t.Fatalf("EnvKeys = %v, want [A B]", owned.EnvKeys)
	}
	if owned.EnvKeysVersion != "v99.0.0" {
		t.Fatalf("EnvKeysVersion bumped backwards to %q", owned.EnvKeysVersion)
	}
}

func TestRenderEnvStep_HeldKeyGoneFromFileDropsFromEnvKeys(t *testing.T) {
	p, _, m, owned := envStepFixture(t)
	owned.EnvKeys = []string{"A", "B", "GONE"}
	var out bytes.Buffer
	if err := renderEnvStep(&out, m, "x.yaml", p, owned, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(owned.EnvKeys, []string{"A", "B"}) {
		t.Fatalf("EnvKeys = %v, want [A B]", owned.EnvKeys)
	}
}

func TestRenderEnvStep_StampsEnvKeysVersion(t *testing.T) {
	p, _, m, _ := envStepFixture(t)
	owned := &managed.Manifest{}
	var out bytes.Buffer
	if err := renderEnvStep(&out, m, "embedded", p, owned, false); err != nil {
		t.Fatal(err)
	}
	if owned.EnvKeysVersion != "dev" {
		t.Fatalf("EnvKeysVersion = %q, want dev", owned.EnvKeysVersion)
	}
}

func TestBumpEnvKeysVersion(t *testing.T) {
	for _, c := range []struct {
		current, recorded string
		want              bool
	}{
		{"dev", "", true},
		{"dev", "dev", true},
		{"v0.41.0", "v0.40.0", true},
		{"v0.40.0", "v0.40.0", true},
		{"v0.40.0", "dev", true},
		{"v0.40.0", "garbage", true},
		{"v0.39.0", "v0.40.0", false},
		{"dev", "v0.40.0", false},
		{"garbage", "v0.40.0", false},
	} {
		if got := bumpEnvKeysVersion(c.current, c.recorded); got != c.want {
			t.Errorf("bump(%q,%q) = %v, want %v", c.current, c.recorded, got, c.want)
		}
	}
}

// Fix A through `up --apply`: the manifest source reaches renderEnvStep, so a
// non-embedded manifest never deletes a recorded key and keeps tracking it.
func TestRunApply_NonEmbeddedManifestKeepsRecordedKey(t *testing.T) {
	t.Setenv("ZENIFY_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	ws := t.TempDir()
	envPath := filepath.Join(ws, ".zenify", ".env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte("A='fake-a'\nDROPPED='fake-dropped'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mp := filepath.Join(ws, ".zenify", "manifest.json")
	if err := (&managed.Manifest{EnvKeys: []string{"A", "DROPPED"}, EnvKeysVersion: "dev"}).Save(mp); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Org: "MyOrg", SecretKeys: []manifest.SecretKey{{Name: "A", Secret: true}}}
	var out bytes.Buffer
	if err := runApply(&out, &out, nil, m, "old/repos.yaml", ws, &fakeGH{}, &fakeGit{}); err != nil {
		t.Fatalf("runApply: %v", err)
	}
	if !strings.Contains(out.String(), "env: giữ DROPPED — manifest không phải bản nhúng (old/repos.yaml)") { //znf:allow-lang
		t.Fatalf("output = %q", out.String())
	}
	if b, _ := os.ReadFile(envPath); !strings.Contains(string(b), "DROPPED='fake-dropped'") { //nolint:gosec // G304 -- test temp path
		t.Fatalf("DROPPED value lost: %q", b)
	}
	owned, err := managed.Load(mp)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(owned.EnvKeys, []string{"A", "DROPPED"}) {
		t.Fatalf("EnvKeys = %v", owned.EnvKeys)
	}
}

// Fix B: the wizard's plan screen carries the same env preview lines as the
// text dry-run, built without writing anything.
func TestWizardPlanFooter_ShowsEnvPreviewWithoutWriting(t *testing.T) {
	t.Setenv("ZENIFY_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	ws := t.TempDir()
	envPath := filepath.Join(ws, ".zenify", ".env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		t.Fatal(err)
	}
	orig := []byte("A='fake-a'\nB='fake-b'\n")
	if err := os.WriteFile(envPath, orig, 0o600); err != nil {
		t.Fatal(err)
	}
	mp := filepath.Join(ws, ".zenify", "manifest.json")
	if err := (&managed.Manifest{EnvKeys: []string{"A", "B"}, EnvKeysVersion: "dev"}).Save(mp); err != nil {
		t.Fatal(err)
	}
	origManifest, _ := os.ReadFile(mp) //nolint:gosec // G304 -- test temp path
	m := &manifest.Manifest{SecretKeys: []manifest.SecretKey{{Name: "A", Secret: true}}}

	rows := wizardPlanFooter(m, "embedded", ws)
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "env: xoá B") { //znf:allow-lang
		t.Fatalf("rows lack the env removal: %q", joined)
	}
	if !strings.Contains(joined, "HOOKS") {
		t.Fatalf("rows lost the plan footer: %q", joined)
	}
	if strings.Contains(joined, "fake-") {
		t.Fatalf("rows leak a value: %q", joined)
	}
	if got, _ := os.ReadFile(envPath); !bytes.Equal(got, orig) { //nolint:gosec // G304 -- test temp path
		t.Fatalf(".env changed: %q", got)
	}
	if got, _ := os.ReadFile(mp); !bytes.Equal(got, origManifest) { //nolint:gosec // G304 -- test temp path
		t.Fatalf("manifest.json changed: %q", got)
	}
	if rows := wizardPlanFooter(m, "old/repos.yaml", ws); !strings.Contains(strings.Join(rows, "\n"), "env: giữ B") { //znf:allow-lang
		t.Fatalf("non-embedded rows = %q", rows)
	}
}
