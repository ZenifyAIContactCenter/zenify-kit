package skilllint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan_FlagsOmitModelOutsideFences(t *testing.T) {
	root := t.TempDir()
	body := "# skill\n\nScale up = **omit `model`** so it inherits.\n\n```\nAgent({ subagent_type: 'x' }) // omitting model here is fine\n```\n\nPass `model: 'sonnet'` for small diffs.\nOmitted model inherits the session.\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("omit model"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := Scan([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 2 || fs[0].Line != 3 || fs[1].Line != 10 {
		t.Fatalf("findings = %+v, want lines 3 and 10", fs)
	}
}
