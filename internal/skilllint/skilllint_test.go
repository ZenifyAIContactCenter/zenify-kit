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

func TestScanSize_FlagsOnlyOversizedSkillMd(t *testing.T) {
	root := t.TempDir()
	small := filepath.Join(root, "small")
	big := filepath.Join(root, "big")
	tall := filepath.Join(root, "tall")
	for _, d := range []string{small, big, tall} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(small, "SKILL.md"), []byte("# ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(big, "SKILL.md"), bytesOf(SizeBytes+1, 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tall, "SKILL.md"), bytesOf(SizeLines+1, SizeLines+1), 0o644); err != nil {
		t.Fatal(err)
	}
	// A big reference file is not a SKILL.md and must not be flagged.
	if err := os.WriteFile(filepath.Join(big, "notes.md"), bytesOf(SizeBytes*2, 1), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := ScanSize([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 2 {
		t.Fatalf("findings = %+v, want big and tall", fs)
	}
	for _, f := range fs {
		if filepath.Dir(f.File) == small {
			t.Fatalf("small skill flagged: %+v", f)
		}
	}
}

// bytesOf returns n bytes containing exactly lines newlines.
func bytesOf(n, lines int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	for i := 0; i < lines && i < n; i++ {
		b[i] = '\n'
	}
	return b
}
