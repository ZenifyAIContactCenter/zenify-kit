package cli

import "testing"

func TestNewSkillsCmdHasSync(t *testing.T) {
	c := newSkillsCmd()
	if c.Use != "skills" {
		t.Fatalf("Use=%q, want skills", c.Use)
	}
	var found bool
	for _, sub := range c.Commands() {
		if sub.Use == "sync" {
			found = true
		}
	}
	if !found {
		t.Fatal("thiếu subcommand sync")
	}
}
