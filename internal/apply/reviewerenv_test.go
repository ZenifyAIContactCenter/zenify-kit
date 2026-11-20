package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureReviewerEnv_CreatesThenNoop(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureReviewerEnv(UserSettings(home), false)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if v, ok := readEnvKey(t, home, ReviewerEnv); !ok || v != ReviewerDefault {
		t.Fatalf("env = %q ok=%v", v, ok)
	}
	if changed, err := EnsureReviewerEnv(UserSettings(home), false); err != nil || changed {
		t.Fatalf("second run changed=%v err=%v", changed, err)
	}
}

func TestEnsureReviewerEnv_KeepsUserValueAndForeignKeys(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"permissions":{"allow":["Bash(ls)"]},"env":{"FOO":"bar","ZNF_REVIEWER":"codex"}}`)
	if changed, err := EnsureReviewerEnv(UserSettings(home), false); err != nil || changed {
		t.Fatalf("must not override: changed=%v err=%v", changed, err)
	}
	if v, _ := readEnvKey(t, home, ReviewerEnv); v != "codex" {
		t.Fatalf("user value rewritten: %q", v)
	}
	if v, _ := readEnvKey(t, home, "FOO"); v != "bar" {
		t.Fatalf("foreign key lost: %q", v)
	}
}

func TestEnsureReviewerEnv_MalformedFailOpen(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{not json`)
	if changed, err := EnsureReviewerEnv(UserSettings(home), false); err == nil || changed {
		t.Fatalf("malformed must error and not write: changed=%v err=%v", changed, err)
	}
}

func TestEnsureReviewerEnv_DryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	if changed, err := EnsureReviewerEnv(UserSettings(home), true); err != nil || !changed {
		t.Fatalf("dry-run reports change: changed=%v err=%v", changed, err)
	}
	if _, ok := readEnvKeyIfExists(t, home, ReviewerEnv); ok {
		t.Fatal("dry-run must not write")
	}
}

func TestRemoveReviewerEnv_OnlyDefault(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"env":{"ZNF_REVIEWER":"agent+project","KEEP":"1"}}`)
	if removed, err := RemoveReviewerEnv(UserSettings(home), false); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if _, ok := readEnvKey(t, home, ReviewerEnv); ok {
		t.Fatal("default value must be removed")
	}
	writeSettings(t, home, `{"env":{"ZNF_REVIEWER":"codex"}}`)
	if removed, err := RemoveReviewerEnv(UserSettings(home), false); err != nil || removed {
		t.Fatalf("user value must stay: removed=%v err=%v", removed, err)
	}
}

func TestSetReviewerEnvRejectsUnknown(t *testing.T) {
	home := t.TempDir()
	if err := SetReviewerEnv(UserSettings(home), "gpt", false); err == nil {
		t.Fatal("unknown value must error")
	}
	if _, ok := readEnvKeyIfExists(t, home, ReviewerEnv); ok {
		t.Fatal("nothing must be written")
	}
	if err := SetReviewerEnv(UserSettings(home), "codex", false); err != nil {
		t.Fatal(err)
	}
	if v, _ := readEnvKey(t, home, ReviewerEnv); v != "codex" {
		t.Fatalf("env = %q", v)
	}
}

func TestCarryEnv_CopiesUserValueOnlyWhenTargetLacksKey(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	writeSettings(t, home, `{"env":{"ZNF_REVIEWER":"codex","CLAUDE_CODE_SUBAGENT_MODEL":"opus"}}`)
	local := WorkspaceLocalSettings(ws)
	if changed, err := CarryEnv(local, UserSettings(home), ReviewerEnv, false); err != nil || !changed {
		t.Fatalf("carry: changed=%v err=%v", changed, err)
	}
	if v, err := ReviewerEnvValue(local); err != nil || v != "codex" {
		t.Fatalf("local reviewer = %q err=%v", v, err)
	}
	// the seed after the carry keeps the carried value
	if changed, err := EnsureReviewerEnv(local, false); err != nil || changed {
		t.Fatalf("seed overwrote carried value: changed=%v err=%v", changed, err)
	}
	// a key the target already has is never overwritten
	if err := SetReviewerEnv(local, "agent", false); err != nil {
		t.Fatal(err)
	}
	if changed, err := CarryEnv(local, UserSettings(home), ReviewerEnv, false); err != nil || changed {
		t.Fatalf("carry over existing: changed=%v err=%v", changed, err)
	}
	// nothing to carry: user file lacks the key, or is absent
	if changed, err := CarryEnv(local, filepath.Join(home, "none.json"), SubagentModelEnv, false); err != nil || changed {
		t.Fatalf("absent source: changed=%v err=%v", changed, err)
	}
}

func TestSetReviewerEnv_AcceptsProject(t *testing.T) {
	home := t.TempDir()
	if err := SetReviewerEnv(UserSettings(home), "project", false); err != nil {
		t.Fatal(err)
	}
	if v, _ := readEnvKey(t, home, ReviewerEnv); v != "project" {
		t.Fatalf("env = %q", v)
	}
}

func TestReviewerEnvValue_Project(t *testing.T) {
	home := t.TempDir()
	if err := SetReviewerEnv(UserSettings(home), "project", false); err != nil {
		t.Fatal(err)
	}
	v, err := ReviewerEnvValue(UserSettings(home))
	if err != nil || v != "project" {
		t.Fatalf("got (%q,%v)", v, err)
	}
}

func TestReviewerDefault_IsAgentPlusProject(t *testing.T) {
	home := t.TempDir()
	if changed, err := EnsureReviewerEnv(UserSettings(home), false); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if v, _ := readEnvKey(t, home, ReviewerEnv); v != "agent+project" {
		t.Fatalf("env = %q", v)
	}
}

func TestReviewerEnv_AcceptsAgentPlusProject(t *testing.T) {
	home := t.TempDir()
	if err := SetReviewerEnv(UserSettings(home), "agent+project", false); err != nil {
		t.Fatal(err)
	}
	v, err := ReviewerEnvValue(UserSettings(home))
	if err != nil || v != "agent+project" {
		t.Fatalf("got (%q,%v)", v, err)
	}
}

func readRecordList(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var l []string
	if err := json.Unmarshal(raw, &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestMigrateReviewerDefault(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "ws", ".claude", "settings.local.json")
	record := filepath.Join(dir, "kit-reviewer-default.json")
	ws := filepath.Join(dir, "ws")
	if err := os.MkdirAll(filepath.Dir(settings), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"env":{"ZNF_REVIEWER":"agent"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// dry-run writes nothing
	if changed, err := MigrateReviewerDefault(settings, record, true); err != nil || !changed {
		t.Fatalf("dry-run changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(record); err == nil {
		t.Fatal("dry-run wrote the record")
	}
	if v, _ := ReviewerEnvValue(settings); v != "agent" {
		t.Fatalf("dry-run changed value: %q", v)
	}
	// agent -> agent+project, record has the workspace
	if changed, err := MigrateReviewerDefault(settings, record, false); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if v, _ := ReviewerEnvValue(settings); v != "agent+project" {
		t.Fatalf("value = %q", v)
	}
	if l := readRecordList(t, record); len(l) != 1 || l[0] != ws {
		t.Fatalf("record = %v", l)
	}
	// user sets agent again: second run keeps it
	if err := SetReviewerEnv(settings, "agent", false); err != nil {
		t.Fatal(err)
	}
	if changed, err := MigrateReviewerDefault(settings, record, false); err != nil || changed {
		t.Fatalf("second run changed=%v err=%v", changed, err)
	}
	if v, _ := ReviewerEnvValue(settings); v != "agent" {
		t.Fatalf("user value overwritten: %q", v)
	}
}

func TestMigrateReviewerDefault_CodexUntouchedButRecorded(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "ws", ".claude", "settings.local.json")
	record := filepath.Join(dir, "kit-reviewer-default.json")
	ws := filepath.Join(dir, "ws")
	if err := os.MkdirAll(filepath.Dir(settings), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"env":{"ZNF_REVIEWER":"codex"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, err := MigrateReviewerDefault(settings, record, false); err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if v, _ := ReviewerEnvValue(settings); v != "codex" {
		t.Fatalf("value = %q", v)
	}
	if l := readRecordList(t, record); len(l) != 1 || l[0] != ws {
		t.Fatalf("record = %v", l)
	}
}
