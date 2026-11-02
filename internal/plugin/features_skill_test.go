package plugin

import "testing"

// SC-38, FR-15.1..FR-15.3: the features skill toggles kit features through `zenify feature` only.
func TestFeaturesSkill(t *testing.T) {
	assertContainsAll(t, "features/SKILL.md", syncedAsset(t, "skills/features/SKILL.md"), []string{
		"name: features",
		"allowed-tools: Bash(zenify feature*)",
		"zenify feature <key> on|off",
		"verbatim",
		"status line",
	})
}
