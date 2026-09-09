package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// readWSEnv reads the env block of settings.local.json in the test workspace.
func readWSEnv(t *testing.T, ws string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json")) //nolint:gosec // G304 -- path is computed internally from t.TempDir, not externally-tainted input
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var root struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return root.Env
}

func writeWSSettings(t *testing.T, ws, body string) {
	t.Helper()
	dir := filepath.Join(ws, ".claude")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.local.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// SC-15: only empty/missing keys go to prompt-set; keys with a value are kept as-is.
func TestSecretStep_OnlyPromptsEmpty_PreservesFilled(t *testing.T) {
	ws := t.TempDir()
	writeWSSettings(t, ws, `{"env":{"MONGO_URL":"mongodb://live","E2E_EMAIL":""}}`)
	var asked []string
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: []string{"MONGO_URL", "E2E_EMAIL", "E2E_PASSWORD"},
		SecretPromptFn: func(keys []string) (map[string]string, error) {
			asked = keys
			return map[string]string{"E2E_EMAIL": "a@b.com", "E2E_PASSWORD": "pw"}, nil
		},
	}
	if err := secretStep(cfg); err != nil {
		t.Fatalf("secretStep: %v", err)
	}
	// MONGO_URL already has a value → must NOT be prompted.
	for _, k := range asked {
		if k == "MONGO_URL" {
			t.Fatalf("MONGO_URL already has a value but was still prompted: %v", asked)
		}
	}
	env := readWSEnv(t, ws)
	if env["MONGO_URL"] != "mongodb://live" {
		t.Errorf("MONGO_URL was changed: %q", env["MONGO_URL"])
	}
	if env["E2E_EMAIL"] != "a@b.com" || env["E2E_PASSWORD"] != "pw" {
		t.Errorf("new value was not written: %+v", env)
	}
}

// SC-16: blank input → keeps the empty placeholder, no overwrite.
func TestSecretStep_BlankInputKeepsPlaceholder(t *testing.T) {
	ws := t.TempDir()
	writeWSSettings(t, ws, `{"env":{"E2E_EMAIL":""}}`)
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: []string{"E2E_EMAIL"},
		SecretPromptFn: func(keys []string) (map[string]string, error) {
			return map[string]string{"E2E_EMAIL": ""}, nil // dev types blank
		},
	}
	if err := secretStep(cfg); err != nil {
		t.Fatalf("secretStep: %v", err)
	}
	if got := readWSEnv(t, ws)["E2E_EMAIL"]; got != "" {
		t.Errorf("blank input but key changed: %q", got)
	}
}

// SC-17: Accessible → no-op, file untouched.
func TestSecretStep_AccessibleNoOp(t *testing.T) {
	ws := t.TempDir()
	writeWSSettings(t, ws, `{"env":{"E2E_EMAIL":""}}`)
	before, _ := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json")) //nolint:gosec // G304 -- path is computed internally from t.TempDir, not externally-tainted input
	called := false
	cfg := OnboardConfig{
		Workspace:  ws,
		Accessible: true,
		SecretKeys: []string{"E2E_EMAIL"},
		SecretPromptFn: func(keys []string) (map[string]string, error) {
			called = true
			return nil, nil
		},
	}
	if err := secretStep(cfg); err != nil {
		t.Fatalf("secretStep: %v", err)
	}
	if called {
		t.Error("Accessible=true but still prompted")
	}
	after, _ := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json")) //nolint:gosec // G304 -- path is computed internally from t.TempDir, not externally-tainted input
	if string(before) != string(after) {
		t.Error("file changed in Accessible mode")
	}
}

// SC-18: value containing & / < is preserved byte-for-byte (SetEscapeHTML off).
func TestSecretStep_DoesNotEscapeSpecialChars(t *testing.T) {
	ws := t.TempDir()
	writeWSSettings(t, ws, `{"env":{"MONGO_URL":""}}`)
	const url = "mongodb://h:27017/db?a=1&b=2&tls=true"
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: []string{"MONGO_URL"},
		SecretPromptFn: func(keys []string) (map[string]string, error) {
			return map[string]string{"MONGO_URL": url}, nil
		},
	}
	if err := secretStep(cfg); err != nil {
		t.Fatalf("secretStep: %v", err)
	}
	if got := readWSEnv(t, ws)["MONGO_URL"]; got != url {
		t.Errorf("value was mangled: %q != %q", got, url)
	}
	raw, _ := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json")) //nolint:gosec // G304 -- path is computed internally from t.TempDir, not externally-tainted input
	if !strContains(string(raw), "a=1&b=2") {
		t.Errorf("bytes on disk were escaped: %s", raw)
	}
}

// Secret-loss guard: readEnvBlock must validate (malformed JSON / env not an
// object) and return an error BEFORE secretStep prompts or writes anything — locking in the
// read-validate-before-write order so a future accidental reorder can't lose a secret.
func TestSecretStep_CorruptJSON_LeavesFileUnchanged(t *testing.T) {
	ws := t.TempDir()
	writeWSSettings(t, ws, `{"env": {`)
	before, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json")) //nolint:gosec // G304 -- path is computed internally from t.TempDir, not externally-tainted input
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	prompted := false
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: []string{"MONGO_URL"},
		SecretPromptFn: func(keys []string) (map[string]string, error) {
			prompted = true
			return map[string]string{"MONGO_URL": "mongodb://should-not-write"}, nil
		},
	}
	if err := secretStep(cfg); err == nil {
		t.Fatal("secretStep: expected error on corrupt JSON, got nil")
	}
	if prompted {
		t.Error("SecretPromptFn was called even though malformed JSON must fail before prompting")
	}
	after, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json")) //nolint:gosec // G304 -- path is computed internally from t.TempDir, not externally-tainted input
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("file changed on malformed JSON: before=%q after=%q", before, after)
	}
}

func TestSecretStep_NonObjectEnv_LeavesFileUnchanged(t *testing.T) {
	ws := t.TempDir()
	writeWSSettings(t, ws, `{"env":"oops"}`)
	before, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json")) //nolint:gosec // G304 -- path is computed internally from t.TempDir, not externally-tainted input
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	prompted := false
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: []string{"MONGO_URL"},
		SecretPromptFn: func(keys []string) (map[string]string, error) {
			prompted = true
			return map[string]string{"MONGO_URL": "mongodb://should-not-write"}, nil
		},
	}
	if err := secretStep(cfg); err == nil {
		t.Fatal("secretStep: expected error when env is not an object, got nil")
	}
	if prompted {
		t.Error("SecretPromptFn was called even though a non-object env must fail before prompting")
	}
	after, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json")) //nolint:gosec // G304 -- path is computed internally from t.TempDir, not externally-tainted input
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("file changed when env isn't an object: before=%q after=%q", before, after)
	}
}

func strContains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
