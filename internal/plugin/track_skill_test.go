package plugin

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/skilllint"
)

// FR-4.1..4.4, SC-13, SC-14, SC-16: the /znf:track skill names every verb, the CLI it calls and the id rules.
func TestTrackSkill(t *testing.T) {
	s := syncedAsset(t, "skills/track/SKILL.md")
	assertContainsAll(t, "track/SKILL.md", s, []string{
		"name: track", "argument-hint:", "\"track\"", "\"backlog\"", "\"plan today\"", "\"next task\"",
		"zenify track end <id> --done", "zenify track end <id> --cancelled --rounds 0", "zenify track est <id>",
		"zenify track plan add", "zenify track plan rm", "zenify track backlog drop", "--kind task",
		"zenify track backlog add", "zenify plane create", "available after PR2",
		"Exactly one `running` item", "two or more `running` items", "no `running` item",
		"no verb or an unknown verb → print help, run no write command", "unknown verb: <verb>",
		"b81e", "#581", "<repo>#581", "ZEN-288",
	})
	for _, v := range []string{"add", "drop", "ls", "plan", "plan +", "plan -", "go", "done", "cancel", "est", "help"} {
		if !strings.Contains(s, "| `"+v) {
			t.Errorf("help table lacks verb %q", v)
		}
	}
	if n := strings.Count(s, "/znf:track "); n < 11 {
		t.Errorf("want an example per verb (>=11 uses of `/znf:track `), got %d", n)
	}
	assertProjectAgnostic(t, "track/SKILL.md", s)
}

func TestTrackSkillSize(t *testing.T) {
	dest := t.TempDir()
	if _, err := Sync(dest, filepath.Join(dest, ".manifest.json")); err != nil {
		t.Fatal(err)
	}
	f, err := skilllint.ScanSize([]string{filepath.Join(dest, "skills", "track")})
	if err != nil {
		t.Fatal(err)
	}
	if len(f) != 0 {
		t.Errorf("size findings: %+v", f)
	}
}
