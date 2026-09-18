package plugin

import (
	"bytes"
	"errors"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/skilllint"
)

var htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)

// Skill budget (token-diet spec FR-4.1, 2026-09-18). SKILL.md is injected
// verbatim into the main context on first invocation, so its size is paid on
// every later turn of that session. 8192 bytes ≈ 2k tokens at ~4.2 bytes/token
// for English markdown; rationale goes to references/ and is read on demand.
// The same caps are enforced at lint time by skilllint.SizeCap / SizeLines so
// `zenify rules lint` fails before `go test` does.
const (
	maxSkillLines = skilllint.SizeLines
	maxSkillBytes = skilllint.SizeBytes
)

func skillDirs(t *testing.T) []string {
	t.Helper()
	root := path.Join(embedRoot, "skills")
	entries, err := fs.ReadDir(assets, root)
	if err != nil {
		t.Fatalf("list %s: %v", root, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := fs.Stat(assets, path.Join(root, e.Name(), "SKILL.md")); errors.Is(err, fs.ErrNotExist) {
			continue // _shared and other auxiliary dirs
		}
		out = append(out, path.Join(root, e.Name()))
	}
	// Floor is an intentional tripwire, pinned to the shipped count as of W5
	// (2026-09-13) — not a target to grow toward. Lowering it is a conscious
	// unship decision; raising it back after a skill count drop just to make
	// the test pass defeats the tripwire.
	if len(out) < 25 {
		t.Fatalf("expected ≥25 skills, found %d", len(out))
	}
	return out
}

func TestSkillBodyBudget(t *testing.T) {
	for _, dir := range skillDirs(t) {
		b, err := assets.ReadFile(path.Join(dir, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		lines := bytes.Count(b, []byte("\n"))
		if lines > maxSkillLines || len(b) > maxSkillBytes {
			t.Errorf("%s/SKILL.md is %d lines / %d bytes; budget is %d lines / %d bytes — move rationale into references/ (token-diet spec FR-4.1)",
				path.Base(dir), lines, len(b), maxSkillLines, maxSkillBytes)
		}
	}
}

func TestSkillReferencesLinkedOneLevel(t *testing.T) {
	for _, dir := range skillDirs(t) {
		refs, err := fs.ReadDir(assets, path.Join(dir, "references"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		body := readAsset(t, path.Join(dir, "SKILL.md"))
		if !strings.Contains(body, "\n## References") {
			t.Errorf("%s has references/ but no '## References' section in SKILL.md", path.Base(dir))
		}
		for _, r := range refs {
			if r.IsDir() || !strings.HasSuffix(r.Name(), ".md") {
				continue
			}
			if !strings.Contains(body, "references/"+r.Name()) {
				t.Errorf("%s/references/%s is not linked from SKILL.md (one-level rule)", path.Base(dir), r.Name())
			}
			ref := readAsset(t, path.Join(dir, "references", r.Name()))
			stripped := htmlCommentRe.ReplaceAllString(ref, "")
			if strings.Contains(stripped, "references/") {
				t.Errorf("%s/references/%s references \"references/\" — references must be one level deep (no nested reference links)", path.Base(dir), r.Name())
			}
		}
	}
}
