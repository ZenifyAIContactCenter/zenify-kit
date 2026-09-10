package plugin

import (
	"os"
	"strings"
	"testing"
)

func TestGroundAndShipWireTwoTierGate(t *testing.T) {
	g, err := os.ReadFile("assets/znf/skills/ground/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	s, err := os.ReadFile("assets/znf/skills/ship/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(g), "mandatory") || !strings.Contains(string(g), "db-perf") {
		t.Fatal("ground SKILL.md must mark the DB-perf gate mandatory")
	}
	if !strings.Contains(string(s), "BLOCKING") || !strings.Contains(string(s), "does not complete") {
		t.Fatal("ship SKILL.md must block completion on a BLOCKING finding")
	}
}

func TestCookStep3ReferencesGate(t *testing.T) {
	c, err := os.ReadFile("assets/znf/skills/cook/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(c), "znf:explain-plan") || !strings.Contains(string(c), "mandatory") {
		t.Fatal("cook Step 3 must reference the mandatory explain-plan gate")
	}
}
