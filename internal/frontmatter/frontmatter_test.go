package frontmatter

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScan(t *testing.T) {
	dir := t.TempDir()

	// wrong key: flagged, on its own line
	write(t, dir, "bad.md", "---\nglobs: **/be/**\n---\n# body\n")
	// correct key: clean
	write(t, dir, "good.md", "---\npaths:\n  - \"**/be/**\"\n---\n# body\n")
	// no frontmatter: clean even though the body mentions globs:
	write(t, dir, "plain.md", "# title\n\nsome prose about globs: here\n")
	// globs only in body, real frontmatter uses paths: clean
	write(t, dir, "bodyonly.md", "---\npaths:\n  - \"x\"\n---\ntext globs: not a key\n")
	// blank lines before frontmatter still parsed
	write(t, dir, "leadblank.md", "\n\n---\nglobs: y\n---\n")

	got, err := Scan([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 findings, got %d: %+v", len(got), got)
	}
	byFile := map[string]int{}
	for _, f := range got {
		byFile[filepath.Base(f.File)] = f.Line
	}
	if byFile["bad.md"] != 2 {
		t.Errorf("bad.md: want line 2, got %d", byFile["bad.md"])
	}
	if byFile["leadblank.md"] != 4 {
		t.Errorf("leadblank.md: want line 4, got %d", byFile["leadblank.md"])
	}
	if _, ok := byFile["good.md"]; ok {
		t.Error("good.md should not be flagged")
	}
	if _, ok := byFile["bodyonly.md"]; ok {
		t.Error("bodyonly.md should not be flagged")
	}
}
