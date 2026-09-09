package langgate

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScan_FlagsVietnameseComment(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a.go", "package a\n// đây là comment tiếng Việt\nvar X = 1\n")
	v, err := Scan([]string{d}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 || v[0].Line != 2 {
		t.Fatalf("want 1 violation at line 2, got %+v", v)
	}
}

func TestScan_EnglishGoClean(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a.go", "package a\n// this is an English comment — with em dash\nvar X = 1\n")
	v, _ := Scan([]string{d}, true)
	if len(v) != 0 {
		t.Fatalf("English + em-dash must be clean, got %+v", v)
	}
}

func TestScan_AllowLangMarkerExempts(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a.go", "package a\nfunc f() { fmt.Println(\"Manifest trống\") } //znf:allow-lang\n")
	v, _ := Scan([]string{d}, true)
	if len(v) != 0 {
		t.Fatalf("allow-lang line must be exempt, got %+v", v)
	}
}

func TestScan_MdCodeFenceExempt(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a.md", "English prose here\n```\n'Nhập tiêu đề'\n```\nmore English\n")
	v, _ := Scan([]string{d}, false)
	if len(v) != 0 {
		t.Fatalf("code fence must be exempt, got %+v", v)
	}
}

func TestScan_MdInlineCodeExempt(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a.md", "The literal `'Nhập tiêu đề'` is a data string.\n")
	v, _ := Scan([]string{d}, false)
	if len(v) != 0 {
		t.Fatalf("inline code must be exempt, got %+v", v)
	}
}

func TestScan_MdProseVietnameseFlagged(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a.md", "Đây là prose tiếng Việt ngoài code.\n")
	v, _ := Scan([]string{d}, false)
	if len(v) != 1 {
		t.Fatalf("VN prose in .md must be flagged, got %+v", v)
	}
}

func TestScan_GoSkippedWhenIncludeGoFalse(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a.go", "package a\n// tiếng Việt\n")
	v, _ := Scan([]string{d}, false)
	if len(v) != 0 {
		t.Fatalf("go must be skipped when includeGo=false, got %+v", v)
	}
}
