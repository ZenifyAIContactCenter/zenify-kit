package docsgen

import (
	"fmt"
	"io/fs"
	"path"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
)

// maxDescriptionRunes is the kit's cap on a skill/agent description. Every
// description is loaded into every session, so a long one costs on every
// turn; the agent-skills spec allows 1024, the kit keeps them to triggers.
const maxDescriptionRunes = 300

// descriptionsOverCap returns "<path>: <n> runes" for every skills/*/SKILL.md
// (skipping _* directories) and agents/*.md whose description exceeds max.
func descriptionsOverCap(fsys fs.FS, max int) ([]string, error) {
	var files []string
	skills, err := fs.Glob(fsys, "skills/*/SKILL.md")
	if err != nil {
		return nil, err
	}
	for _, p := range skills {
		if strings.HasPrefix(path.Base(path.Dir(p)), "_") {
			continue
		}
		files = append(files, p)
	}
	agents, err := fs.Glob(fsys, "agents/*.md")
	if err != nil {
		return nil, err
	}
	files = append(files, agents...)

	var over []string
	for _, p := range files {
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, err
		}
		fm, err := ParseFrontmatter(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if n := utf8.RuneCountInString(fm.Description); n > max {
			over = append(over, fmt.Sprintf("%s: %d runes", p, n))
		}
	}
	return over, nil
}

func TestDescriptionsOverCap_Synthetic(t *testing.T) {
	md := func(name, desc string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("---\nname: " + name + "\ndescription: " + desc + "\n---\nbody\n")}
	}
	fsys := fstest.MapFS{
		"skills/a/SKILL.md":  md("a", strings.Repeat("ơ", 301)), //znf:allow-lang
		"skills/_x/SKILL.md": md("x", strings.Repeat("ơ", 400)), //znf:allow-lang
		"agents/b.md":        md("b", strings.Repeat("ơ", 300)), //znf:allow-lang
	}
	got, err := descriptionsOverCap(fsys, maxDescriptionRunes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.Contains(got[0], "skills/a/SKILL.md") || !strings.Contains(got[0], "301") {
		t.Fatalf("want one entry for skills/a/SKILL.md at 301 runes, got %q", got)
	}
}

func TestDescriptionsUnderCap_RealEmbeddedTree(t *testing.T) {
	got, err := descriptionsOverCap(plugin.ZnfFS(), maxDescriptionRunes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 0 {
		t.Fatalf("descriptions over %d runes:\n%s", maxDescriptionRunes, strings.Join(got, "\n"))
	}
}
