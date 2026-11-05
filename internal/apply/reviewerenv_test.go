package apply

import (
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
	writeSettings(t, home, `{"env":{"ZNF_REVIEWER":"agent","KEEP":"1"}}`)
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
