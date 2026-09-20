package plugin

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

var znfSkillRefRe = regexp.MustCompile("`znf:([a-z0-9-]+)`")

// Every `znf:<x>` named in _shared/skill-routing.md must be a real skill dir — a name that
// drifts from its directory is the #1 failure mode in the spec (Skill(...) → Unknown skill).
func TestSkillRoutingNamesResolve(t *testing.T) {
	body := readAsset(t, path.Join(embedRoot, "skills", "_shared", "skill-routing.md"))
	seen := map[string]bool{}
	for _, m := range znfSkillRefRe.FindAllStringSubmatch(body, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		if _, err := fs.Stat(assets, path.Join(embedRoot, "skills", name, "SKILL.md")); err != nil {
			t.Errorf("skill-routing.md names znf:%s but %s/SKILL.md does not exist", name, name)
		}
	}
	for _, want := range []string{"mongo-data-safety", "sql-data-safety", "mongoose-modeling",
		"service-integration", "express-service-patterns", "nestjs-patterns", "react-patterns"} {
		if !seen[want] {
			t.Errorf("skill-routing.md must route to znf:%s", want)
		}
	}
	for _, want := range []string{"-conventions", "Unknown skill", "_Skills:"} {
		if !strings.Contains(body, want) {
			t.Errorf("skill-routing.md missing %q", want)
		}
	}
}

// SC-6: the five workflow files cite the shared routing reference.
func TestSkillRoutingCited(t *testing.T) {
	for _, rel := range []string{
		"skills/writing-plans/SKILL.md",
		"skills/ground/SKILL.md",
		"skills/discipline/SKILL.md",
		"skills/using-zenify-kit/SKILL.md",
		"skills/subagent-driven-development/implementer-prompt.md",
	} {
		if !strings.Contains(readAsset(t, path.Join(embedRoot, rel)), "skill-routing") {
			t.Errorf("%s does not cite _shared/skill-routing", rel)
		}
	}
}
