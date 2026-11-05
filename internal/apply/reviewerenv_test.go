package apply

import "testing"

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
