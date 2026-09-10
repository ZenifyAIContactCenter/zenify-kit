package dbperf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsThresholds(t *testing.T) {
	d := Defaults()
	if d.LargeCount != 100000 || d.ScanRatioBlock != 100 || d.ScanRatioAdvisory != 10 || d.SkipLarge != 1000 {
		t.Fatalf("unexpected defaults: %+v", d)
	}
}

func TestLoadMergesOverDefaults(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/db-collections.json"
	os.WriteFile(p, []byte(`{"tenant_scoped":["tickets","chat_rooms"],"global":["configs"],"skip_large":500}`), 0o600)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.TenantScoped) != 2 || c.Global[0] != "configs" {
		t.Fatalf("lists not loaded: %+v", c)
	}
	if c.SkipLarge != 500 || c.LargeCount != 100000 { // overridden vs default-kept
		t.Fatalf("threshold merge wrong: %+v", c)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file must be non-error (fail-open): %v", err)
	}
	if c.LargeCount != 100000 {
		t.Fatal("missing file should yield defaults")
	}
}
