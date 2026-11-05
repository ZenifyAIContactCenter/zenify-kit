package plugin

import (
	"io/fs"
	"strings"
	"testing"
)

// FR-1.5: the kit's skill tree no longer lives under ~/.claude/skills/znf, so no
// embedded asset may point a reader there.
func TestAssetsHaveNoUserSkillsPath(t *testing.T) {
	banned := []string{"~/.claude/skills/znf", ".claude/skills/znf/"}
	err := fs.WalkDir(assets, embedRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(assets, p)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, bad := range banned {
				if strings.Contains(line, bad) {
					t.Errorf("%s:%d contains %q: %s", p, i+1, bad, line)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
