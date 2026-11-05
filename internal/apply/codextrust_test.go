package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCodexTrust_Append(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	orig := "# mine\n[mcp_servers.x]\ncommand = \"x\"\n"
	if err := os.WriteFile(cfg, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureCodexTrust(home, "/ws", false)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	b, _ := os.ReadFile(cfg)
	got := string(b)
	if !strings.HasPrefix(got, orig) {
		t.Fatalf("original bytes altered: %q", got)
	}
	if !strings.Contains(got, "[projects.\"/ws\"]\ntrust_level = \"trusted\"\n") {
		t.Fatalf("missing trust table: %q", got)
	}
	changed, err = EnsureCodexTrust(home, "/ws", false)
	if err != nil || changed {
		t.Fatalf("second call changed=%v err=%v", changed, err)
	}
}

func TestEnsureCodexTrust_FlipUntrusted(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	in := "[projects.\"/ws\"]\ntrust_level = \"untrusted\"\n\n[other]\na = 1\n"
	if err := os.WriteFile(cfg, []byte(in), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureCodexTrust(home, "/ws", false)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	b, _ := os.ReadFile(cfg)
	want := "[projects.\"/ws\"]\ntrust_level = \"trusted\"\n\n[other]\na = 1\n"
	if string(b) != want {
		t.Fatalf("got %q", b)
	}
}

func TestEnsureCodexTrust_CreatesFileAndDryRun(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureCodexTrust(home, "/ws", true)
	if err != nil || !changed {
		t.Fatalf("dry changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); err == nil {
		t.Fatal("dry run wrote a file")
	}
	if _, err := EnsureCodexTrust(home, "/ws", false); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(home, "config.toml"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("stat %v %v", st, err)
	}
}

func TestRemoveCodexTrust_RoundTrip(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	orig := "# mine\n[mcp_servers.x]\ncommand = \"x\"\n"
	_ = os.WriteFile(cfg, []byte(orig), 0o600)
	if _, err := EnsureCodexTrust(home, "/ws", false); err != nil {
		t.Fatal(err)
	}
	changed, err := RemoveCodexTrust(home, "/ws", false)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	b, _ := os.ReadFile(cfg)
	if string(b) != orig {
		t.Fatalf("got %q", b)
	}
	changed, _ = RemoveCodexTrust(home, "/ws", false)
	if changed {
		t.Fatal("second remove should be no-op")
	}
}

func TestRemoveCodexTrust_KeepsForeignTable(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	in := "[projects.\"/ws\"]\ntrust_level = \"trusted\"\nfoo = \"bar\"\n"
	_ = os.WriteFile(cfg, []byte(in), 0o600)
	changed, err := RemoveCodexTrust(home, "/ws", false)
	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	b, _ := os.ReadFile(cfg)
	if string(b) != in {
		t.Fatalf("got %q", b)
	}
}
