package cli

import (
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

// FR-7.3: a repo-scoped key is never asked at onboarding, even when that repo is selected.
func TestWizardSecretKeys_GlobalOnly(t *testing.T) {
	m := &manifest.Manifest{
		SecretKeys: []manifest.SecretKey{{Name: "MONGO_URL", Secret: true}, {Name: "E2E_PASSWORD", Secret: true}},
		Repos:      []manifest.Repo{{Name: "lumi-agent", SecretKeys: []manifest.SecretKey{{Name: "OPENROUTER_API_KEY", Secret: true}}}},
	}
	got := manifest.KeyNames(wizardSecretKeys(m))
	want := map[string]bool{"MONGO_URL": true, "E2E_PASSWORD": true}
	if len(got) != len(want) {
		t.Fatalf("keys = %v, want exactly MONGO_URL and E2E_PASSWORD", got)
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("unexpected key asked: %s", k)
		}
	}
}
