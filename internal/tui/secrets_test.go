package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// readWSEnv đọc env block của settings.local.json trong workspace test.
func readWSEnv(t *testing.T, ws string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json"))
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

// SC-15: chỉ key rỗng/thiếu vào prompt-set; key có value giữ nguyên.
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
	// MONGO_URL đã có value → KHÔNG được hỏi.
	for _, k := range asked {
		if k == "MONGO_URL" {
			t.Fatalf("MONGO_URL đã có value nhưng vẫn bị prompt: %v", asked)
		}
	}
	env := readWSEnv(t, ws)
	if env["MONGO_URL"] != "mongodb://live" {
		t.Errorf("MONGO_URL bị đổi: %q", env["MONGO_URL"])
	}
	if env["E2E_EMAIL"] != "a@b.com" || env["E2E_PASSWORD"] != "pw" {
		t.Errorf("value mới không được ghi: %+v", env)
	}
}

// SC-16: input trống → giữ placeholder rỗng, không đè.
func TestSecretStep_BlankInputKeepsPlaceholder(t *testing.T) {
	ws := t.TempDir()
	writeWSSettings(t, ws, `{"env":{"E2E_EMAIL":""}}`)
	cfg := OnboardConfig{
		Workspace:  ws,
		SecretKeys: []string{"E2E_EMAIL"},
		SecretPromptFn: func(keys []string) (map[string]string, error) {
			return map[string]string{"E2E_EMAIL": ""}, nil // dev gõ trống
		},
	}
	if err := secretStep(cfg); err != nil {
		t.Fatalf("secretStep: %v", err)
	}
	if got := readWSEnv(t, ws)["E2E_EMAIL"]; got != "" {
		t.Errorf("gõ trống nhưng key đổi: %q", got)
	}
}

// SC-17: Accessible → no-op, file bất biến.
func TestSecretStep_AccessibleNoOp(t *testing.T) {
	ws := t.TempDir()
	writeWSSettings(t, ws, `{"env":{"E2E_EMAIL":""}}`)
	before, _ := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json"))
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
		t.Error("Accessible=true nhưng vẫn prompt")
	}
	after, _ := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json"))
	if string(before) != string(after) {
		t.Error("file bị đổi trong chế độ Accessible")
	}
}

// SC-18: value chứa & / < giữ nguyên byte (SetEscapeHTML off).
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
		t.Errorf("value bị mangle: %q != %q", got, url)
	}
	raw, _ := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json"))
	if !strContains(string(raw), "a=1&b=2") {
		t.Errorf("byte trên đĩa bị escape: %s", raw)
	}
}

// Secret-loss guard: readEnvBlock phải validate (JSON hỏng / env không phải
// object) và trả lỗi TRƯỚC khi secretStep prompt hay ghi gì — khoá thứ tự
// read-validate-trước-write để tương lai reorder vô tình không làm mất secret.
func TestSecretStep_CorruptJSON_LeavesFileUnchanged(t *testing.T) {
	ws := t.TempDir()
	writeWSSettings(t, ws, `{"env": {`)
	before, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json"))
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
		t.Error("SecretPromptFn được gọi dù JSON hỏng phải fail trước prompt")
	}
	after, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("file bị đổi khi JSON hỏng: before=%q after=%q", before, after)
	}
}

func TestSecretStep_NonObjectEnv_LeavesFileUnchanged(t *testing.T) {
	ws := t.TempDir()
	writeWSSettings(t, ws, `{"env":"oops"}`)
	before, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json"))
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
		t.Error("SecretPromptFn được gọi dù env không phải object phải fail trước prompt")
	}
	after, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("file bị đổi khi env không phải object: before=%q after=%q", before, after)
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
