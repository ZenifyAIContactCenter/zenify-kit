package hub

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteTmuxConf_Golden(t *testing.T) { // FR-12.2, FR-12.3
	home := t.TempDir()
	path, err := WriteTmuxConf(home, "/Users/John Doe/it's/#zen$ify")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".zenify", "hub", "tmux.conf"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "tmux.conf.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("tmux.conf:\n%s\nwant:\n%s", got, want)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
		}
	}
}

func TestWriteTmuxConf_Overwrites(t *testing.T) {
	home := t.TempDir()
	p, err := WriteTmuxConf(home, "/old/zenify")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("user edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteTmuxConf(home, "/new/zenify"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := tmuxConf("/new/zenify"); string(b) != want {
		t.Errorf("not overwritten:\n%s", b)
	}
}
