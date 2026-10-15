package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

// readEnvFile returns the key→value map of <ws>/.zenify/.env.
func readEnvFile(t *testing.T, ws string) map[string]string {
	t.Helper()
	f, err := devenv.Load(devenv.Path(ws))
	if err != nil {
		t.Fatalf("load env file: %v", err)
	}
	return f.Values()
}

func writeEnvFileFor(t *testing.T, ws, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(devenv.Path(ws), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func secretKeysOf(names ...string) []manifest.SecretKey {
	out := make([]manifest.SecretKey, len(names))
	for i, n := range names {
		out[i] = manifest.SecretKey{Name: n, Secret: true}
	}
	return out
}

// SC-15: only empty/missing keys go to prompt-set; keys with a value are kept as-is.
func TestSecretStep_OnlyPromptsEmpty_PreservesFilled(t *testing.T) {
	ws := t.TempDir()
	writeEnvFileFor(t, ws, "MONGO_URL='mongodb://live'\nE2E_EMAIL=\n")
	var asked []string
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: secretKeysOf("MONGO_URL", "E2E_EMAIL", "E2E_PASSWORD"),
		SecretPromptFn: func(names []string) (map[string]string, error) {
			asked = names
			return map[string]string{"E2E_EMAIL": "a@b.com", "E2E_PASSWORD": "pw"}, nil
		},
	}
	if err := secretStep(cfg); err != nil {
		t.Fatalf("secretStep: %v", err)
	}
	for _, k := range asked {
		if k == "MONGO_URL" {
			t.Fatalf("MONGO_URL already has a value but was still prompted: %v", asked)
		}
	}
	env := readEnvFile(t, ws)
	if env["MONGO_URL"] != "mongodb://live" {
		t.Errorf("MONGO_URL was changed: %q", env["MONGO_URL"])
	}
	if env["E2E_EMAIL"] != "a@b.com" || env["E2E_PASSWORD"] != "pw" {
		t.Errorf("new value was not written: %+v", env)
	}
}

// The wizard writes .zenify/.env (0600) and leaves settings.local.json alone.
func TestSecretStep_WritesEnvFile(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(ws, ".claude", "settings.local.json")
	const settingsBody = `{"env":{"MONGO_URL":""}}`
	if err := os.WriteFile(settings, []byte(settingsBody), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: secretKeysOf("MONGO_URL"),
		SecretPromptFn: func([]string) (map[string]string, error) {
			return map[string]string{"MONGO_URL": "mongodb://fake"}, nil
		},
	}
	if err := secretStep(cfg); err != nil {
		t.Fatalf("secretStep: %v", err)
	}
	if got := readEnvFile(t, ws)["MONGO_URL"]; got != "mongodb://fake" {
		t.Errorf("env file MONGO_URL = %q", got)
	}
	fi, err := os.Stat(devenv.Path(ws))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("env file mode = %v err=%v, want 0600", fi.Mode().Perm(), err)
	}
	after, _ := os.ReadFile(settings) //nolint:gosec // G304 -- path is computed internally from t.TempDir
	if string(after) != settingsBody {
		t.Errorf("settings.local.json changed: %q", after)
	}
}

// SC-16: blank input → nothing written.
func TestSecretStep_BlankInputWritesNothing(t *testing.T) {
	ws := t.TempDir()
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: secretKeysOf("E2E_EMAIL"),
		SecretPromptFn: func([]string) (map[string]string, error) {
			return map[string]string{"E2E_EMAIL": ""}, nil
		},
	}
	if err := secretStep(cfg); err != nil {
		t.Fatalf("secretStep: %v", err)
	}
	if _, err := os.Stat(devenv.Path(ws)); !os.IsNotExist(err) {
		t.Errorf("blank input must not create the env file, stat err=%v", err)
	}
}

// SC-17: Accessible → no-op, file untouched.
func TestSecretStep_AccessibleNoOp(t *testing.T) {
	ws := t.TempDir()
	called := false
	cfg := OnboardConfig{
		Workspace:  ws,
		Accessible: true,
		SecretKeys: secretKeysOf("E2E_EMAIL"),
		SecretPromptFn: func([]string) (map[string]string, error) {
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
	if _, err := os.Stat(devenv.Path(ws)); !os.IsNotExist(err) {
		t.Error("env file created in Accessible mode")
	}
}

// A value with '&' or a quote survives the round trip through the file.
func TestSecretStep_SpecialCharsRoundTrip(t *testing.T) {
	ws := t.TempDir()
	const url = "mongodb://h:27017/db?a=1&b=2&tls=true&x='y'"
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: secretKeysOf("MONGO_URL"),
		SecretPromptFn: func([]string) (map[string]string, error) {
			return map[string]string{"MONGO_URL": url}, nil
		},
	}
	if err := secretStep(cfg); err != nil {
		t.Fatalf("secretStep: %v", err)
	}
	if got := readEnvFile(t, ws)["MONGO_URL"]; got != url {
		t.Errorf("value was mangled: %q != %q", got, url)
	}
}

// A value holding a newline is refused before anything is written.
func TestSecretStep_NewlineRefused(t *testing.T) {
	ws := t.TempDir()
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: secretKeysOf("MONGO_URL"),
		SecretPromptFn: func([]string) (map[string]string, error) {
			return map[string]string{"MONGO_URL": "a\nINJECTED=1"}, nil
		},
	}
	if err := secretStep(cfg); err == nil {
		t.Fatal("expected error for a newline value")
	}
	if _, err := os.Stat(devenv.Path(ws)); !os.IsNotExist(err) {
		t.Error("env file written despite the refusal")
	}
}

// FR-4.4: production keys are never prompted for.
func TestSecretStep_SkipsProduction(t *testing.T) {
	ws := t.TempDir()
	writeEnvFileFor(t, ws, "E2E_EMAIL=\nMONGO_URL_PRD=\n")
	keys := []manifest.SecretKey{
		{Name: "E2E_EMAIL", Secret: true, Group: manifest.GroupTooling},
		{Name: "MONGO_URL_PRD", Secret: true, Group: manifest.GroupProduction},
	}
	var asked []string
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: keys,
		SecretPromptFn: func(names []string) (map[string]string, error) {
			asked = names
			return map[string]string{}, nil
		},
	}
	if err := secretStep(cfg); err != nil {
		t.Fatalf("secretStep: %v", err)
	}
	if len(asked) != 1 || asked[0] != "E2E_EMAIL" {
		t.Fatalf("want only E2E_EMAIL prompted, got %v", asked)
	}
}
