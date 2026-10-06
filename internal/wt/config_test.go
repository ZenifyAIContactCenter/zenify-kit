package wt

import (
	"os"
	"path/filepath"
	"testing"
)

func writeWorktreeJSON(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worktree.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_ArrayPortRangeAndDefaults(t *testing.T) {
	root := t.TempDir()
	writeWorktreeJSON(t, root, `{"abbrev":"cch","baseRef":"origin/staging","portRange":[3250,3299],"deps":"clone"}`)
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.Abbrev != "cch" || c.BaseRef != "origin/staging" {
		t.Fatalf("scalar wrong: %+v", c)
	}
	if c.PortRange != [2]int{3250, 3299} {
		t.Fatalf("array portRange wrong: %v", c.PortRange)
	}
	// defaults for missing keys
	if c.WorktreeDir != ".worktrees/" || c.PortEnv != "PORT" || c.User != "" {
		t.Fatalf("defaults wrong: %+v", c)
	}
}

func TestLoad_StringPortRangeLegacy(t *testing.T) {
	root := t.TempDir()
	writeWorktreeJSON(t, root, `{"portRange":"3100 3999"}`)
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.PortRange != [2]int{3100, 3999} {
		t.Fatalf("string portRange wrong: %v", c.PortRange)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("want error for missing worktree.json")
	}
}

func TestLoad_NullPortRangeUsesDefault(t *testing.T) {
	root := t.TempDir()
	writeWorktreeJSON(t, root, `{"portRange":null}`)
	c, err := Load(root)
	if err != nil {
		t.Fatalf("explicit null portRange must fall back to default, got err: %v", err)
	}
	if c.PortRange != [2]int{3100, 3999} {
		t.Fatalf("null portRange wrong: %v", c.PortRange)
	}
}

func TestConfigGateHotfixStanza(t *testing.T) {
	dir := t.TempDir()
	body := `{
	  "abbrev":"lumi","baseRef":"origin/staging",
	  "hotfix":{"baseStrategy":"standalone"},
	  "gate":{"sharedStore":false}
	}`
	writeWorktreeJSON(t, dir, body)
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HotfixBaseStrategy != "standalone" {
		t.Fatalf("baseStrategy=%q, want standalone", c.HotfixBaseStrategy)
	}
	if c.GateSharedStore {
		t.Fatal("lumi-agent must have sharedStore=false")
	}
}

func TestConfigHotfixDefaultsStaging(t *testing.T) {
	dir := t.TempDir()
	writeWorktreeJSON(t, dir, `{"abbrev":"x"}`)
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HotfixBaseStrategy != "staging" {
		t.Fatalf("default baseStrategy=%q, want staging", c.HotfixBaseStrategy)
	}
}

func TestConfigDepsDirDefault(t *testing.T) {
	dir := t.TempDir()
	writeWorktreeJSON(t, dir, `{"abbrev":"x"}`) // depsDir not declared
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.DepsDir != "node_modules" {
		t.Fatalf("DepsDir default = %q, want node_modules", c.DepsDir)
	}
}

func TestConfigDepsDirCustom(t *testing.T) {
	dir := t.TempDir()
	writeWorktreeJSON(t, dir, `{"abbrev":"x","depsDir":"vendor"}`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.DepsDir != "vendor" {
		t.Fatalf("DepsDir = %q, want vendor", c.DepsDir)
	}
}

// The branch's <username> must come from the dev running wt, never a
// hardcoded name: declared > user.email local part > $USER.
func TestBranchUser_Resolution(t *testing.T) {
	email := fakeRunner{out: map[string]string{"/r|config --get user.email": "quyentm@zenify.vn\n"}}
	if got := BranchUser(email, "/r", "lead"); got != "lead" {
		t.Errorf("declared user must win, got %q", got)
	}
	if got := BranchUser(email, "/r", ""); got != "quyentm" {
		t.Errorf("want email local part, got %q", got)
	}
	noEmail := fakeRunner{err: map[string]error{"/r|config --get user.email": errFake}}
	t.Setenv("USERNAME", "")
	t.Setenv("USER", "alice")
	if got := BranchUser(noEmail, "/r", ""); got != "alice" {
		t.Errorf("want $USER fallback, got %q", got)
	}
}

// Characters git rejects in a ref must not reach the branch name, and an
// unusable candidate falls through to the next source.
func TestBranchUser_SanitizesAndFallsThrough(t *testing.T) {
	t.Setenv("USER", "")
	t.Setenv("USERNAME", "Win User")
	cases := map[string]string{
		"a~b:c@x.com":    "a-b-c",
		".first..last@x": "first.last",
		"name.lock@x":    "name",
		"~~~@x":          "Win-User", // cleans to empty → $USERNAME (Windows)
	}
	for email, want := range cases {
		r := fakeRunner{out: map[string]string{"/r|config --get user.email": email}}
		if got := BranchUser(r, "/r", ""); got != want {
			t.Errorf("email %q: got %q, want %q", email, got, want)
		}
	}
}
