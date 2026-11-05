package apply

import (
	"os"
	"testing"
)

func TestRemoveStrongModelEnv_AnyValue(t *testing.T) {
	for _, val := range []string{"fable", "opus"} {
		home := t.TempDir()
		writeSettings(t, home, `{"env":{"ZNF_STRONG_MODEL":"`+val+`","OTHER":"x"}}`)
		if removed, err := RemoveStrongModelEnv(UserSettings(home), false); err != nil || !removed {
			t.Fatalf("%s: removed=%v err=%v", val, removed, err)
		}
		if _, ok := readEnvKey(t, home, StrongModelEnv); ok {
			t.Fatalf("%s: key must be removed", val)
		}
		if v, _ := readEnvKey(t, home, "OTHER"); v != "x" {
			t.Fatalf("%s: foreign key lost: %q", val, v)
		}
	}
}

func TestRemoveStrongModelEnv_DryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"env":{"ZNF_STRONG_MODEL":"fable"}}`)
	if removed, err := RemoveStrongModelEnv(UserSettings(home), true); err != nil || !removed {
		t.Fatalf("dry-run reports change: removed=%v err=%v", removed, err)
	}
	if v, ok := readEnvKey(t, home, StrongModelEnv); !ok || v != "fable" {
		t.Fatalf("dry-run must not write: %q ok=%v", v, ok)
	}
}

func TestRemoveStrongModelEnv_MalformedFailOpen(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{not json`)
	if removed, err := RemoveStrongModelEnv(UserSettings(home), false); err == nil || removed {
		t.Fatalf("malformed must error and not write: removed=%v err=%v", removed, err)
	}
}

func readEnvKeyIfExists(t *testing.T, home, key string) (string, bool) {
	t.Helper()
	if _, err := os.Stat(settingsPath(home)); err != nil {
		return "", false
	}
	return readEnvKey(t, home, key)
}
