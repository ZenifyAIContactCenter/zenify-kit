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

// ship --watch hands off to ci-watch, and ci-watch hands a failed run to fix instead of fixing or
// pushing itself — the diagnosis and the push stay behind fix's loop and ship's gate.
func TestShipWatchWiresCIWatchToFix(t *testing.T) {
	s, err := os.ReadFile("assets/znf/skills/ship/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(s), "`--watch` → `Skill(znf:ci-watch)`") {
		t.Fatal("ship SKILL.md must invoke znf:ci-watch on --watch")
	}
	c, err := os.ReadFile("assets/znf/skills/ci-watch/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Skill(znf:fix)", "CI round <n>/2", "pushes nothing itself"} {
		if !strings.Contains(string(c), want) {
			t.Fatalf("ci-watch SKILL.md missing %q", want)
		}
	}
	for _, banned := range []string{"--log-failed", "git push"} {
		if strings.Contains(string(c), banned) {
			t.Fatalf("ci-watch SKILL.md must not do fix's or ship's job (%q)", banned)
		}
	}
}
