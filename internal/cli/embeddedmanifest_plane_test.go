package cli

import (
	"testing"

	kit "github.com/ZenifyAIContactCenter/zenify-kit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

func TestEmbeddedManifest_Plane(t *testing.T) {
	m, err := manifest.Parse(kit.DefaultRepos, "embedded")
	if err != nil {
		t.Fatal(err)
	}
	if m.Plane == nil || m.Plane.URL != "https://plane.zenify.vn" || m.Plane.Workspace != "zenify" {
		t.Fatalf("Plane = %+v", m.Plane)
	}
	var n int
	for _, k := range m.SecretKeys {
		if k.Name != "PLANE_API_KEY" {
			continue
		}
		n++
		if !k.Secret || k.Scope != "personal" || k.EffectiveGroup() != "tooling" {
			t.Errorf("PLANE_API_KEY = %+v", k)
		}
	}
	if n != 1 {
		t.Fatalf("PLANE_API_KEY count = %d, want 1", n)
	}
}
