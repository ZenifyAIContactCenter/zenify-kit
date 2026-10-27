package plugin

import (
	"strings"
	"testing"
)

// FR-3.1..FR-3.9, SC-7, SC-8: with github.issues on, ship links an issue (Closes #N) before the PR.
func TestShipGithubIssues(t *testing.T) {
	ref := syncedAsset(t, "skills/ship/references/github-issues.md")
	assertContainsAll(t, "github-issues.md", ref, []string{
		"features: github.issues on",
		"Closes #",
		"gh issue create --assignee @me",
		"gh issue list --state open --search",
		"gh pr list --state open",
		"--body-file",
		"Features: github.issues on",
		"<owner>/<repo>#N",
		"top layer",
		"default branch",
	})
	if strings.Contains(ref, "references/") {
		t.Error("github-issues.md must not contain references/")
	}
	assertContainsAll(t, "ship/SKILL.md", syncedAsset(t, "skills/ship/SKILL.md"),
		[]string{"references/github-issues.md"})
	assertContainsAll(t, "pr-message-language.md", syncedAsset(t, "skills/ship/references/pr-message-language.md"),
		[]string{"Closes #N"})
	assertContainsAll(t, "stacked-ship.md", syncedAsset(t, "skills/ship/references/stacked-ship.md"),
		[]string{"github-issues.md"})
}
