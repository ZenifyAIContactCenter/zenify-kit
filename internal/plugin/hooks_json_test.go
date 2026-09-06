package plugin

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// hooks.json phải hợp lệ và có Stop + SessionStart chạy `zenify docs sync`.
func TestHooksJSONHasDocsSync(t *testing.T) {
	b, err := os.ReadFile("assets/znf/hooks/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatalf("hooks.json không parse: %v", err)
	}
	if !strings.Contains(string(b), "zenify docs sync") {
		t.Fatal("thiếu lệnh 'zenify docs sync'")
	}
	hooks, _ := root["hooks"].(map[string]any)
	if _, ok := hooks["Stop"]; !ok {
		t.Fatal("thiếu hook Stop")
	}
	if _, ok := hooks["SessionStart"]; !ok {
		t.Fatal("thiếu hook SessionStart")
	}
}
