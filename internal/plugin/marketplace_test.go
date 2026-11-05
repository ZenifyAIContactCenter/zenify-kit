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
	if err := json.Unmarshal([]byte(`{"name":"zenify-kit","owner":{"name":"zenify"},"plugins":[{"name":"znf","source":"./plugins/znf"}]}`), &want); err != nil {
		t.Fatal(err)
	}
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

func TestSyncMarketplaceRestoresStalePluginJSON(t *testing.T) {
	zh := t.TempDir()
	pj := filepath.Join(PluginDir(zh), ".claude-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(pj), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pj, []byte(`{"name":"znf","version":"0.0.1","userConfig":{"old":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := SyncMarketplace(zh)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range res.Kept {
		if k == pj {
			t.Fatalf("plugin.json reported as Kept: %v", res.Kept)
		}
	}
	raw, err := os.ReadFile(pj)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["userConfig"]; !ok || string(got["userConfig"]) == `{"old":{}}` {
		t.Fatalf("userConfig not restored from embed: %s", raw)
	}
	if string(got["version"]) != `"`+version.Current()+`"` {
		t.Fatalf("version = %s", got["version"])
	}
	if res2, err := SyncMarketplace(zh); err != nil || len(res2.Written) != 0 {
		t.Fatalf("not idempotent: %v %v", res2.Written, err)
	}
}
