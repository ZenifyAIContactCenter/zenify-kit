package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

// FR-3.4, FR-3.4a, FR-12.1: the shared reference names every track command and the off switch.
func TestTrackSharedDoc(t *testing.T) {
	s := syncedAsset(t, "skills/_shared/track.md")
	assertContainsAll(t, "track.md", s, []string{
		"zenify track start", "zenify track est", "zenify track ship", "zenify track fix",
		"zenify track ci", "zenify track end", "--watch", "--rollback", "--branch", "track: off",
		"--blocked-step", "znf:ci-watch",
	})
}

// FR-12.2: the four skills already at the byte cap are not touched.
func TestTrackLeavesCappedSkillsAlone(t *testing.T) {
	sizes := map[string]int64{"cook": 8187, "fix": 8191, "hotfix": 8171, "ship": 8182}
	for name, want := range sizes {
		fi, err := os.Stat(filepath.Join("assets", "znf", "skills", name, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Size() != want {
			t.Errorf("%s/SKILL.md = %d bytes, want %d", name, fi.Size(), want)
		}
	}
}
