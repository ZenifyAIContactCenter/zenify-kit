package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFeatureCmd_OnThenList(t *testing.T) {
	ws := t.TempDir()
	out, _, err := execCmd(newFeatureCmd(), "github.issues", "on", "--workspace", ws)
	if err != nil {
		t.Fatal(err)
	}
	if out != "github.issues on\n" {
		t.Fatalf("set output = %q", out)
	}
	out, _, err = execCmd(newFeatureCmd(), "--workspace", ws)
	if err != nil {
		t.Fatal(err)
	}
	if out != "github.issues on\ntrack off\n" {
		t.Fatalf("list output = %q", out)
	}
}

func TestFeatureCmd_DefaultOff(t *testing.T) {
	out, _, err := execCmd(newFeatureCmd(), "--workspace", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out != "github.issues off\ntrack off\n" {
		t.Fatalf("output = %q", out)
	}
}

func TestFeatureCmd_BadValue(t *testing.T) {
	ws := t.TempDir()
	if _, _, err := execCmd(newFeatureCmd(), "github.issues", "yes", "--workspace", ws); err == nil {
		t.Fatal("want error for value yes")
	}
	if _, err := os.Stat(filepath.Join(ws, ".zenify-overlay.yaml")); !os.IsNotExist(err) {
		t.Fatalf("overlay must not exist, stat err = %v", err)
	}
	if _, _, err := execCmd(newFeatureCmd(), "github.issues", "--workspace", ws); err == nil {
		t.Fatal("want error for single arg")
	}
}

func TestFeatureCmd_UnknownKey(t *testing.T) {
	if _, _, err := execCmd(newFeatureCmd(), "foo", "on", "--workspace", t.TempDir()); err == nil {
		t.Fatal("want error for unknown key")
	}
}

func TestFeatureCmd_TrackOnThenList(t *testing.T) {
	ws := t.TempDir()
	out, _, err := execCmd(newFeatureCmd(), "track", "on", "--workspace", ws)
	if err != nil || out != "track on\n" {
		t.Fatalf("set output = %q err %v", out, err)
	}
	out, _, err = execCmd(newFeatureCmd(), "--workspace", ws)
	if err != nil || out != "github.issues off\ntrack on\n" {
		t.Fatalf("list output = %q err %v", out, err)
	}
}
