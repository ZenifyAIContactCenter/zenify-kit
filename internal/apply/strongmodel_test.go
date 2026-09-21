package apply

import (
	"os"
	"testing"
)

func TestEnsureStrongModelEnv_CreatesThenNoop(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureStrongModelEnv(home, false)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if v, ok := readEnvKey(t, home, StrongModelEnv); !ok || v != StrongModelDefault {
		t.Fatalf("env = %q ok=%v", v, ok)
	}
	if changed, err := EnsureStrongModelEnv(home, false); err != nil || changed {
		t.Fatalf("second run changed=%v err=%v", changed, err)
	}
}

func TestEnsureStrongModelEnv_KeepsUserValueAndForeignKeys(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"permissions":{"allow":["Bash(ls)"]},"env":{"FOO":"bar","ZNF_STRONG_MODEL":"fable"}}`)
	if changed, err := EnsureStrongModelEnv(home, false); err != nil || changed {
		t.Fatalf("must not override: changed=%v err=%v", changed, err)
	}
	if v, _ := readEnvKey(t, home, StrongModelEnv); v != "fable" {
		t.Fatalf("user value rewritten: %q", v)
	}
	if v, _ := readEnvKey(t, home, "FOO"); v != "bar" {
		t.Fatalf("foreign key lost: %q", v)
	}
}

func TestEnsureStrongModelEnv_MalformedFailOpen(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{not json`)
	if changed, err := EnsureStrongModelEnv(home, false); err == nil || changed {
		t.Fatalf("malformed must error and not write: changed=%v err=%v", changed, err)
	}
}

func TestEnsureStrongModelEnv_DryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	if changed, err := EnsureStrongModelEnv(home, true); err != nil || !changed {
		t.Fatalf("dry-run reports change: changed=%v err=%v", changed, err)
	}
	if _, ok := readEnvKeyIfExists(t, home, StrongModelEnv); ok {
		t.Fatal("dry-run must not write")
	}
}

func TestRemoveStrongModelEnv_OnlyDefault(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"env":{"ZNF_STRONG_MODEL":"opus","KEEP":"1"}}`)
	if removed, err := RemoveStrongModelEnv(home, false); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if _, ok := readEnvKey(t, home, StrongModelEnv); ok {
		t.Fatal("default value must be removed")
	}
	writeSettings(t, home, `{"env":{"ZNF_STRONG_MODEL":"fable"}}`)
	if removed, err := RemoveStrongModelEnv(home, false); err != nil || removed {
		t.Fatalf("user value must stay: removed=%v err=%v", removed, err)
	}
}

func readEnvKeyIfExists(t *testing.T, home, key string) (string, bool) {
	t.Helper()
	if _, err := os.Stat(settingsPath(home)); err != nil {
		return "", false
	}
	return readEnvKey(t, home, key)
}
