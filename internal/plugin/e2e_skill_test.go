package plugin

import (
	"io/fs"
	"strings"
	"testing"
)

func TestE2eSkillEmbedded(t *testing.T) {
	b, err := fs.ReadFile(assets, "assets/znf/skills/e2e/SKILL.md")
	if err != nil {
		t.Fatalf("SKILL.md e2e phải được embed: %v", err)
	}
	if !strings.Contains(string(b), "@domain-assert") {
		t.Fatal("SKILL.md phải dạy marker @domain-assert")
	}
}
