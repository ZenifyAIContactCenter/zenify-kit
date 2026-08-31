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
	if err := os.WriteFile(f, []byte("content-v1"), 0o644); err != nil {
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
