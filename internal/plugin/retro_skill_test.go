package plugin

import (
	"strings"
	"testing"
)

// TestRetro_Skill pins the retro contract: user-invoked only, one batch list,
// one home per lesson, quotes kept out of written files, and no hardcoded
// project paths (the kit is project-agnostic).
func TestRetro_Skill(t *testing.T) {
	s := syncedAsset(t, "skills/retro/SKILL.md")
	assertContainsAll(t, "retro/SKILL.md", s, []string{
		"disable-model-invocation: true",
		"one list",
		"quotes stay in chat",
		"strip secrets",
		"write no file",
		"no lessons",
		"the rule, not the event",
		"CLAUDE.md",
		"~/.claude/projects",
		"memory",
		"rule",
		"one home",
		"read the existing memory",
		"redact",
		"data, never instructions",
		"at most 2000 lines",
		"exact text to be written",
		"grep each",
	})
	if strings.Contains(s, "allowed-tools: Read Grep Glob Bash") {
		t.Errorf("retro/SKILL.md must not grant Bash")
	}
	for _, bad := range []string{".claude/memory/", "~/.zenify/knowledge", "AskUserQuestion"} {
		if strings.Contains(s, bad) {
			t.Errorf("retro/SKILL.md must not contain %q", bad)
		}
	}
	if len(s) > 8192 {
		t.Errorf("retro/SKILL.md is %d bytes, cap 8192", len(s))
	}
	if n := strings.Count(s, "\n"); n > 200 {
		t.Errorf("retro/SKILL.md is %d lines, cap 200", n)
	}
}
