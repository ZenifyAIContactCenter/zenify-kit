package docsgen

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestGenSkills_EscapesAngleBracketsAndPipes covers the symmetric-escaping
// fix: a skill description containing "<slug>" and "a|b" must come out
// backslash-escaped in the page body (angle brackets only) and in the index
// table cell (angle brackets AND the pipe, since a bare "|" would split the
// column).
func TestGenSkills_EscapesAngleBracketsAndPipes(t *testing.T) {
	znf := fstest.MapFS{
		"skills/escdemo/SKILL.md": &fstest.MapFile{Data: []byte(
			"---\nname: escdemo\ndescription: Dùng cho <slug> và a|b. Chi tiết sau.\n---\nbody\n", //znf:allow-lang
		)},
		"agents/escagent.md": &fstest.MapFile{Data: []byte(
			"---\nname: escagent\ndescription: Agent cho <slug> và a|b. Chi tiết sau.\n---\nbody\n", //znf:allow-lang
		)},
	}
	files, err := GenSkills(znf, fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}

	skillPage := string(files["skills/escdemo.md"])
	if !strings.Contains(skillPage, `\<slug\>`) {
		t.Errorf("skill page body missing escaped angle brackets: %q", skillPage)
	}
	if strings.Contains(skillPage, "<slug>") {
		t.Errorf("skill page body has unescaped angle brackets: %q", skillPage)
	}

	agentPage := string(files["agents/escagent.md"])
	if !strings.Contains(agentPage, `\<slug\>`) {
		t.Errorf("agent page body missing escaped angle brackets: %q", agentPage)
	}

	skillIndex := string(files["skills/index.md"])
	if !strings.Contains(skillIndex, `\<slug\> và a\|b`) { //znf:allow-lang
		t.Errorf("skills index cell missing escaped angle brackets + pipe: %q", skillIndex)
	}

	agentIndex := string(files["agents/index.md"])
	if !strings.Contains(agentIndex, `\<slug\> và a\|b`) { //znf:allow-lang
		t.Errorf("agents index cell missing escaped angle brackets + pipe: %q", agentIndex)
	}
}
