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

func TestRemoveCodexTrust_KeepsUserUntrusted(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	in := "[projects.\"/ws\"]\ntrust_level = \"untrusted\"\n"
	_ = os.WriteFile(cfg, []byte(in), 0o600)
	if changed, err := RemoveCodexTrust(home, "/ws", false); err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if b, _ := os.ReadFile(cfg); string(b) != in {
		t.Fatalf("got %q", b)
	}
}

// Removing a table in the middle keeps exactly one blank line between its
// neighbours.
func TestRemoveCodexTrust_MidFileKeepsOneSeparator(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	in := "[a]\nx = 1\n\n[projects.\"/ws\"]\ntrust_level = \"trusted\"\n\n[b]\ny = 2\n"
	_ = os.WriteFile(cfg, []byte(in), 0o600)
	if changed, err := RemoveCodexTrust(home, "/ws", false); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if b, _ := os.ReadFile(cfg); string(b) != "[a]\nx = 1\n\n[b]\ny = 2\n" {
		t.Fatalf("got %q", b)
	}
}

// A multi-line array line starting with "[" is not the next table header.
func TestEnsureCodexTrust_MultiLineArrayInTable(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	in := "[projects.\"/ws\"]\nextra = [\n  [1, 2],\n]\n"
	_ = os.WriteFile(cfg, []byte(in), 0o600)
	if _, err := EnsureCodexTrust(home, "/ws", false); err != nil {
		t.Fatal(err)
	}
	if changed, _ := RemoveCodexTrust(home, "/ws", false); changed {
		t.Fatal("table with another key must stay")
	}
	b, _ := os.ReadFile(cfg)
	if strings.Count(string(b), "[projects.") != 1 || !strings.Contains(string(b), codexTrustLine) {
		t.Fatalf("got %q", b)
	}
}

func TestEnsureCodexTrust_EscapesPath(t *testing.T) {
	home := t.TempDir()
	ws := `C:\Users\a"b`
	if _, err := EnsureCodexTrust(home, ws, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(home, "config.toml"))
	if !strings.Contains(string(b), `[projects."C:\\Users\\a\"b"]`) {
		t.Fatalf("got %q", b)
	}
	if changed, err := RemoveCodexTrust(home, ws, false); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
}

func TestRemoveCodexTrust_DryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureCodexTrust(home, "/ws", false); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, "config.toml")
	before, _ := os.ReadFile(cfg)
	if changed, err := RemoveCodexTrust(home, "/ws", true); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if after, _ := os.ReadFile(cfg); string(after) != string(before) {
		t.Fatal("dry run wrote")
	}
}

// An indented last array element without a comma is not a table header, so
// the existing trust_level after it is found and flipped, not duplicated.
func TestEnsureCodexTrust_IndentedArrayElementNotHeader(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	in := "[projects.\"/ws\"]\nextra = [\n  [1, 2]\n]\ntrust_level = \"untrusted\"\n"
	_ = os.WriteFile(cfg, []byte(in), 0o600)
	if _, err := EnsureCodexTrust(home, "/ws", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(cfg)
	if strings.Count(string(b), "trust_level") != 1 || !strings.Contains(string(b), codexTrustLine) {
		t.Fatalf("got %q", b)
	}
}

func TestIsTableHeader(t *testing.T) {
	for l, want := range map[string]bool{
		`[a]`:                             true,
		`[[a.b]]`:                         true,
		`[projects."/x"] # [manual] keep`: true,
		`[projects."/a]b"]`:               true,
		`[projects.'C:\x']`:               true,
		`[1, 2]`:                          false,
		`[1, 2],`:                         false,
		`  [1, 2]`:                        false,
		`[[1, 2], [3]]`:                   false,
		`["a"]`:                           true, // a quoted key is a valid header
		`[a] x`:                           false,
		`# [a]`:                           false,
	} {
		if got := isTableHeader(l); got != want {
			t.Errorf("isTableHeader(%q) = %v, want %v", l, got, want)
		}
	}
}

// A `]` in the next table's trailing comment must not hide that header:
// trust goes to the workspace table, the next project is untouched.
func TestEnsureCodexTrust_NextHeaderWithBracketComment(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	in := "[projects.\"/ws\"]\n\n[projects.\"/other\"] # [manual] keep untrusted\ntrust_level = \"untrusted\"\n"
	_ = os.WriteFile(cfg, []byte(in), 0o600)
	if _, err := EnsureCodexTrust(home, "/ws", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(cfg)
	want := "[projects.\"/ws\"]\n" + codexTrustLine + "\n\n[projects.\"/other\"] # [manual] keep untrusted\ntrust_level = \"untrusted\"\n"
	if string(b) != want {
		t.Fatalf("got %q", b)
	}
}
