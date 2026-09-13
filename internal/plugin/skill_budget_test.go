package plugin

import (
	"bytes"
	"errors"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

var htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)

// W4 skill budget. Anthropic's skill-authoring guidance keeps a SKILL.md body
// under 500 lines (docs/authoring/writing-skills/anthropic-best-practices.md, "Token budgets" —
// kit-author reference, not shipped).
// Bytes are the tokenizer-free proxy for the ~5k-token target: English markdown
// runs ~4.2 bytes/token, so 22000 bytes ≈ 5.2k tokens. Measured on the whole
// file — every skill's frontmatter is ≤ 6 lines, so a body split is not needed.
const (
	maxSkillLines = 500
	maxSkillBytes = 22000
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
			t.Errorf("%s/SKILL.md is %d lines / %d bytes; budget is %d lines / %d bytes — move rationale into references/ (see W4 spec)",
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
