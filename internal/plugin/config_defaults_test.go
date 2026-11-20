package plugin

import (
	"encoding/json"
	"testing"
)

// The /config rows' defaults are product decisions: track on, pane off, feedback and issues opt-in.
func TestPluginConfigDefaults(t *testing.T) {
	raw, err := assets.ReadFile("assets/znf/.claude-plugin/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var pj struct {
		UserConfig map[string]struct {
			Default any `json:"default"`
		} `json:"userConfig"`
	}
	if err := json.Unmarshal(raw, &pj); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"track": true, "trackPane": "off", "trackFeedback": false, "githubIssues": false}
	for key, def := range want {
		row, ok := pj.UserConfig[key]
		if !ok {
			t.Errorf("userConfig.%s is missing", key)
			continue
		}
		if row.Default != def {
			t.Errorf("userConfig.%s.default = %v, want %v", key, row.Default, def)
		}
	}
}
