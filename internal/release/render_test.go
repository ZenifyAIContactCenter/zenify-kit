package release

import (
	"strings"
	"testing"
)

func TestRenderRequiredSections(t *testing.T) {
	rep := Report{
		N: 84, GeneratedAt: "2026-09-05 16:10",
		Repos: []RepoReport{{
			Name: "be", Participated: true, PrevRelease: 83, CutDate: "2026-08-26",
			TypeCounts: map[string]int{"fix": 1}, HasMigration: false, HasTestTouch: true,
			SharedHits: []string{"**/chat_*"},
			Regression: []Commit{{SHA: "9dc", Subject: "temporary disable report api"}},
			Hotfixes:   []Commit{{SHA: "aaa", Subject: "Merge ... hotfix/x", Branch: "hungnk/hotfix/x"}},
		}},
		NotShipped:      []string{"notification"},
		SharedCrossRepo: map[string][]string{"**/chat_*": {"be", "chatting"}},
		DeployOrderNote: true,
	}
	out := Render(rep)
	for _, want := range []string{
		"# Release 84", "Ngày cắt", "## be", "Regression", "9dc",
		"Hotfix", "notification", "migration → BE", "**/chat_*",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render thiếu %q\n---\n%s", want, out)
		}
	}
}

func TestRenderEmptyReportNoPanic(t *testing.T) {
	out := Render(Report{N: 84, GeneratedAt: "x", SharedCrossRepo: map[string][]string{}})
	if !strings.Contains(out, "# Release 84") {
		t.Errorf("empty render missing header: %s", out)
	}
}
