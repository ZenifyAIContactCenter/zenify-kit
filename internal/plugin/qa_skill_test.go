package plugin

import (
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/e2e"
)

// TestQASkillContract pins the znf:qa contract (spec lat6e FR-06): the QA run
// writes a frozen scenario before dev code exists, its journey must be red on
// base, and bind never bends the expects. The template's worked example must
// parse with the same parser `zenify e2e lint` uses.
func TestQASkillContract(t *testing.T) {
	s := syncedAsset(t, "skills/qa/SKILL.md")
	assertContainsAll(t, "qa/SKILL.md", s, []string{
		"name: qa",
		"allowed-tools: Bash(zenify *) Bash(git *) Bash(node *) Read Grep Write Edit",
		"scenario ZEN-<n>",
		"journey ZEN-<n>",
		"bind ZEN-<n>",
		"Never read the dev run's branch, plan, diff or commits",
		"references/scenario-template.md",
		"zenify await needs-human",
		"@scenario",
		"@ac:",
		"zenify e2e lint",
		"zenify e2e run --local",
		"red at the `expect` of an `@ac`",
		"Never change a literal inside an `expect`",
		"Never edit the scenario file",
		"e2e.config.json",
		"$ZNF_SANDBOX_WEB_PORT",
		"http://127.0.0.1:3001",
		"contact-center-web",
		"api-commit.mjs",
		"/out/qa-result.json",
		`{"journey":false,"reason":`,
		`{"green":false,"failures":`,
		"Never `git push`",
	})

	tpl := syncedAsset(t, "skills/qa/references/scenario-template.md")
	ex := fencedBlockStarting(tpl, "# ZEN-123:")
	if ex == "" {
		t.Fatal("scenario-template.md has no fenced example starting with `# ZEN-123:`")
	}
	sc, err := e2e.ParseScenario(ex)
	if err != nil {
		t.Fatalf("template example does not parse: %v", err)
	}
	if sc.Key != "ZEN-123" || sc.Role == "" {
		t.Fatalf("bad example header: key=%q role=%q", sc.Key, sc.Role)
	}
	kinds := map[e2e.ACKind]bool{}
	for _, ac := range sc.ACs {
		kinds[ac.Kind] = true
		if ac.Kind == e2e.ACKindE2E && (len(ac.Steps) == 0 || len(ac.Sees) == 0) {
			t.Errorf("e2e @ac:%d needs steps and seen text: %+v", ac.N, ac)
		}
	}
	for _, k := range []e2e.ACKind{e2e.ACKindE2E, e2e.ACKindAsk, e2e.ACKindOut} {
		if !kinds[k] {
			t.Errorf("template example lacks an @ac of kind %q", k)
		}
	}
}

// fencedBlockStarting returns the body of the first ``` fence whose first line
// starts with prefix.
func fencedBlockStarting(doc, prefix string) string {
	parts := strings.Split(doc, "```")
	for i := 1; i < len(parts); i += 2 {
		body := parts[i]
		if nl := strings.IndexByte(body, '\n'); nl >= 0 {
			body = body[nl+1:] // drop the info string line
		}
		if strings.HasPrefix(body, prefix) {
			return body
		}
	}
	return ""
}
