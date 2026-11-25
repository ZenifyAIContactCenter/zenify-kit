package infra

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	c, err := LoadConfig(t.TempDir())
	if err != nil || c.MaxEnvs != 3 || c.ChartRef != "main" {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	home := t.TempDir()
	want := Config{Domain: "a.test", MaxEnvs: 5, ChartRef: "dev"}
	if err := SaveConfig(home, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(home)
	if err != nil || got != want {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestLoadConfigCorrupt(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".zenify", "infra")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(home); err == nil {
		t.Fatal("corrupt config must error")
	}
}
