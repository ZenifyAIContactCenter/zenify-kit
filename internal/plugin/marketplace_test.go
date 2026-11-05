package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

func TestSyncMarketplace(t *testing.T) {
	zh := t.TempDir()
	if _, err := SyncMarketplace(zh); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(zh, "plugin-marketplace")

	raw, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal([]byte(`{"name":"zenify-kit","owner":{"name":"zenify"},"plugins":[{"name":"znf","source":"./plugins/znf"}]}`), &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("marketplace.json = %s", raw)
	}

	raw, err = os.ReadFile(filepath.Join(root, "plugins", "znf", ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pj map[string]any
	if err := json.Unmarshal(raw, &pj); err != nil {
		t.Fatal(err)
	}
	if pj["version"] != version.Current() {
		t.Fatalf("version = %v, want %s", pj["version"], version.Current())
	}
	if pj["name"] != "znf" || pj["userConfig"] == nil {
		t.Fatalf("plugin.json lost original keys: %s", raw)
	}

	if _, err := os.Stat(filepath.Join(root, "plugins", "znf", "skills", "cook", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	res, err := SyncMarketplace(zh)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 0 {
		t.Fatalf("second sync wrote %v", res.Written)
	}
}
