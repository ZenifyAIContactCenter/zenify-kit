package plugin

import (
	"strings"
	"testing"
)

func readAsset(t *testing.T, p string) string {
	t.Helper()
	b, err := assets.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func TestSpecTemplateDocumentsSupersedes(t *testing.T) {
	got := readAsset(t, "assets/znf/skills/_shared/spec-template.md")
	if !strings.Contains(got, "_Supersedes:") {
		t.Fatal("spec-template.md must document the _Supersedes: tag")
	}
}

func TestShipEncouragesSpecTrailer(t *testing.T) {
	got := readAsset(t, "assets/znf/skills/ship/SKILL.md")
	if !strings.Contains(got, "Spec:") {
		t.Fatal("ship SKILL.md must mention the Spec: commit trailer")
	}
}
