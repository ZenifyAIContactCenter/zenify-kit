package apply

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func readEnvKey(t *testing.T, home, key string) (string, bool) {
	t.Helper()
	raw, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var root map[string]map[string]string
	if err := json.Unmarshal(raw, &root); err != nil {
		// other top-level keys may not be objects of strings; decode env only
		var loose map[string]json.RawMessage
		if err := json.Unmarshal(raw, &loose); err != nil {
			t.Fatal(err)
		}
		env := map[string]string{}
		if e, ok := loose["env"]; ok {
			_ = json.Unmarshal(e, &env)
		}
		v, ok := env[key]
		return v, ok
	}
	v, ok := root["env"][key]
	return v, ok
}

func TestEnsureSubagentModelEnv_CreatesWhenFileAbsent(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureSubagentModelEnv(home, false)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if v, ok := readEnvKey(t, home, SubagentModelEnv); !ok || v != SubagentModelDefault {
		t.Fatalf("env = %q ok=%v", v, ok)
	}
	// second run is a no-op
	if changed, err := EnsureSubagentModelEnv(home, false); err != nil || changed {
		t.Fatalf("second run changed=%v err=%v", changed, err)
	}
}

func TestEnsureSubagentModelEnv_RespectsExistingKeyAndPreservesForeign(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"permissions":{"allow":["Bash(ls)"]},"env":{"FOO":"bar","CLAUDE_CODE_SUBAGENT_MODEL":"opus"}}`)
	changed, err := EnsureSubagentModelEnv(home, false)
	if err != nil || changed {
		t.Fatalf("must not override a user value: changed=%v err=%v", changed, err)
	}
	if v, _ := readEnvKey(t, home, SubagentModelEnv); v != "opus" {
		t.Fatalf("user value rewritten: %q", v)
	}

	// key absent, foreign env + permissions kept (the file is re-indented, so
	// compare decoded values, not bytes)
	writeSettings(t, home, `{"permissions":{"allow":["Bash(ls)"]},"env":{"FOO":"bar"}}`)
	if changed, err := EnsureSubagentModelEnv(home, false); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if v, _ := readEnvKey(t, home, "FOO"); v != "bar" {
		t.Fatalf("foreign env lost: %q", v)
	}
	if v, _ := readEnvKey(t, home, SubagentModelEnv); v != SubagentModelDefault {
		t.Fatalf("env = %q", v)
	}
	raw, _ := os.ReadFile(settingsPath(home))
	if !strings.Contains(string(raw), `"Bash(ls)"`) {
		t.Fatalf("permissions lost: %s", raw)
	}
}

func TestEnsureSubagentModelEnv_FailOpen(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{not json`)
	if _, err := EnsureSubagentModelEnv(home, false); err == nil {
		t.Fatal("malformed file must error, not be overwritten")
	}
	writeSettings(t, home, `{"env":"oops"}`)
	if _, err := EnsureSubagentModelEnv(home, false); err == nil {
		t.Fatal("non-object env must error")
	}
	writeSettings(t, home, `{}`)
	if changed, err := EnsureSubagentModelEnv(home, true); err != nil || !changed {
		t.Fatalf("dry run changed=%v err=%v", changed, err)
	}
	if raw, _ := os.ReadFile(settingsPath(home)); string(raw) != `{}` {
		t.Fatalf("dry run wrote: %s", raw)
	}
}

func TestRemoveSubagentModelEnv_RemovesOnlyKitValue(t *testing.T) {
	home := t.TempDir()
	if removed, err := RemoveSubagentModelEnv(home, false); err != nil || removed {
		t.Fatalf("no file: removed=%v err=%v", removed, err)
	}
	writeSettings(t, home, `{"env":{"CLAUDE_CODE_SUBAGENT_MODEL":"sonnet"},"hooks":{}}`)
	removed, err := RemoveSubagentModelEnv(home, false)
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	raw, _ := os.ReadFile(settingsPath(home))
	if strings.Contains(string(raw), `"env"`) || !strings.Contains(string(raw), `"hooks"`) {
		t.Fatalf("empty env must be dropped, hooks kept: %s", raw)
	}

	writeSettings(t, home, `{"env":{"CLAUDE_CODE_SUBAGENT_MODEL":"opus","X":"1"}}`)
	if removed, err := RemoveSubagentModelEnv(home, false); err != nil || removed {
		t.Fatalf("user value must stay: removed=%v err=%v", removed, err)
	}
}
