package plugin

import (
	"strings"
	"testing"
)

func TestDisciplineSkillIsAgnostic(t *testing.T) {
	b, err := assets.ReadFile("assets/znf/skills/discipline/SKILL.md")
	if err != nil {
		t.Fatalf("discipline SKILL.md not embedded: %v", err)
	}
	s := strings.ToLower(string(b))
	for _, f := range []string{
		"herdr", "3csoft", "contact-center", "ott-gateway",
		"db_read", "lumi", "personal-zalo", "notification-hub",
	} {
		if strings.Contains(s, f) {
			t.Errorf("discipline SKILL.md leaks project/personal token %q", f)
		}
	}
	for _, anchor := range []string{"blast radius", "worktree", "fabricat", "verify"} {
		if !strings.Contains(s, anchor) {
			t.Errorf("discipline SKILL.md missing expected anchor %q", anchor)
		}
	}
}

func TestDisciplinePointsToCraft(t *testing.T) {
	b, err := assets.ReadFile("assets/znf/skills/discipline/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "znf:_shared/skill-routing") {
		t.Fatal("discipline does not route to the domain skills via znf:_shared/skill-routing")
	}
}
