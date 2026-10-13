package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

func readAgent(t *testing.T, home, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, "agents", name+".toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSyncCodexAgentsMapsFrontmatter(t *testing.T) {
	home := t.TempDir()
	res, err := SyncCodexAgents(home, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 5 {
		t.Fatalf("written = %v, want 5", res.Written)
	}
	for _, n := range []string{"scout", "code-reviewer", "ui-verifier", "researcher", "spec-reviewer"} {
		s := readAgent(t, home, n)
		if strings.SplitN(s, "\n", 2)[0] != CodexAgentMarker {
			t.Errorf("%s: first line is not marker", n)
		}
		for _, l := range strings.Split(s, "\n") {
			if strings.HasPrefix(l, "model =") {
				t.Errorf("%s: has model key", n)
			}
		}
		var m map[string]any
		if err := toml.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("%s: toml decode: %v", n, err)
		}
		src, _ := assets.ReadFile(embedRoot + "/agents/" + n + ".md")
		_, body, _ := parseAgentFrontmatter(src)
		first := strings.SplitN(strings.TrimLeft(body, "\n"), "\n", 2)[0]
		di, _ := m["developer_instructions"].(string)
		if !strings.HasPrefix(di, first) {
			t.Errorf("%s: developer_instructions starts %q, want %q", n, di[:20], first)
		}
	}
	if !strings.Contains(readAgent(t, home, "code-reviewer"), `sandbox_mode = "read-only"`) {
		t.Error("code-reviewer missing read-only")
	}
	a := readAgent(t, home, "spec-reviewer")
	if !strings.Contains(a, `model_reasoning_effort = "high"`) || strings.Contains(a, "sandbox_mode") {
		t.Error("spec-reviewer mapping wrong")
	}
	// Codex 0.155.1 rejects mcp_servers as a list ("expected a map"); it must not be emitted.
	if strings.Contains(readAgent(t, home, "ui-verifier"), "mcp_servers") {
		t.Error("ui-verifier must not carry a list-form mcp_servers")
	}
}

func TestSyncCodexAgentsIdempotent(t *testing.T) {
	home := t.TempDir()
	if _, err := SyncCodexAgents(home, false); err != nil {
		t.Fatal(err)
	}
	res, err := SyncCodexAgents(home, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 0 || len(res.Unchanged) != 5 {
		t.Fatalf("second run: %+v", res)
	}
}

func TestSyncCodexAgentsDryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	res, err := SyncCodexAgents(home, true)
	if err != nil || len(res.Written) != 5 {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(home, "agents")); !os.IsNotExist(err) {
		t.Error("dry-run created agents dir")
	}
}

func TestSyncCodexAgentsSkipsForeign(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, "agents", "scout.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("name = \"scout\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := SyncCodexAgents(home, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "scout" {
		t.Fatalf("skipped = %v", res.Skipped)
	}
	if b, _ := os.ReadFile(p); string(b) != "name = \"scout\"\n" {
		t.Error("foreign file modified")
	}
}

func TestSyncCodexAgentsPrunesRetiredMarked(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "agents")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	retired := filepath.Join(dir, "architect.toml")
	if err := os.WriteFile(retired, []byte(CodexAgentMarker+"\nname = \"architect\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, "mine.toml")
	if err := os.WriteFile(foreign, []byte("name = \"mine\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if res, err := SyncCodexAgents(home, true); err != nil || len(res.Removed) != 1 {
		t.Fatalf("dry-run: %+v %v", res, err)
	}
	if _, err := os.Stat(retired); err != nil {
		t.Fatal("dry-run removed the retired agent")
	}
	res, err := SyncCodexAgents(home, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 || res.Removed[0] != "architect" {
		t.Fatalf("removed = %v", res.Removed)
	}
	if _, err := os.Stat(retired); !os.IsNotExist(err) {
		t.Error("retired marked agent still on disk")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Error("foreign agent removed")
	}
	if len(res.Written) != 5 {
		t.Errorf("written = %v", res.Written)
	}
}

func TestRemoveCodexAgentsKeepsForeign(t *testing.T) {
	home := t.TempDir()
	if _, err := SyncCodexAgents(home, false); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(home, "agents", "mine.toml")
	if err := os.WriteFile(foreign, []byte("name = \"mine\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveCodexAgents(home, false)
	if err != nil || len(removed) != 5 {
		t.Fatalf("%v %v", removed, err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Error("foreign removed")
	}
}

func TestAgentTOMLEscapesAndRejectsTripleQuote(t *testing.T) {
	out, err := agentTOML("x", agentMeta{Name: "x", Description: `say "hi" \ ok`}, "body\n")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := toml.Unmarshal([]byte(out), &m); err != nil || m["description"] != `say "hi" \ ok` {
		t.Fatalf("%v %v", m, err)
	}
	if _, err := agentTOML("x", agentMeta{Name: "x"}, "a ''' b"); err == nil {
		t.Error("expected error for '''")
	}
}
