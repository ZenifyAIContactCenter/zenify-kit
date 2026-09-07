package managed

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprint_Stable(t *testing.T) {
	a := Fingerprint([]byte("hello"))
	b := Fingerprint([]byte("hello"))
	if a != b || a == "" {
		t.Errorf("fingerprint unstable: %q vs %q", a, b)
	}
}

func TestManifest_RecordAndRoundtrip(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "seed.txt")
	if err := os.WriteFile(f, []byte("content-v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{}
	if err := m.Record(f); err != nil {
		t.Fatalf("record: %v", err)
	}
	mpath := filepath.Join(dir, "manifest.json")
	if err := m.Save(mpath); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(mpath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	e, ok := loaded.Get(f)
	if !ok {
		t.Fatalf("entry for %s missing", f)
	}
	if e.SHA256 != Fingerprint([]byte("content-v1")) {
		t.Errorf("recorded sha %q != fingerprint of content", e.SHA256)
	}
}

func TestManifestVersionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".manifest.json")

	m := &Manifest{Entries: map[string]Entry{}, Version: "v0.9.0"}
	if err := m.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Version != "v0.9.0" {
		t.Fatalf("version = %q, want v0.9.0", got.Version)
	}
}

func TestManifestLoadLegacyNoVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".manifest.json")
	if err := os.WriteFile(path, []byte(`{"entries":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load legacy: %v", err)
	}
	if got.Version != "" {
		t.Fatalf("legacy version = %q, want empty", got.Version)
	}
}
