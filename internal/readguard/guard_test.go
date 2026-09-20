package readguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, name string, size int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func ip(v int) *int { return &v }

func TestDecide(t *testing.T) {
	big := write(t, "big.md", TextLimitBytes+1)
	small := write(t, "small.md", 1000)
	pdf := write(t, "doc.PDF", 10_000)
	png := write(t, "shot.png", ImageLimitBytes+1)
	smallPng := write(t, "icon.png", 100_000)

	cases := []struct {
		name string
		in   Input
		deny bool
		hint string
	}{
		{"big text no range", Input{FilePath: big}, true, "offset"},
		{"big text with range", Input{FilePath: big, Offset: ip(1), Limit: ip(200)}, false, ""},
		{"big text offset only", Input{FilePath: big, Offset: ip(1)}, true, "limit"},
		{"small text", Input{FilePath: small}, false, ""},
		{"pdf no pages", Input{FilePath: pdf}, true, "pages"},
		{"pdf with pages", Input{FilePath: pdf, Pages: "1-3"}, false, ""},
		{"big image", Input{FilePath: png}, true, "ui-verifier"},
		{"small image", Input{FilePath: smallPng}, false, ""},
		{"missing file", Input{FilePath: filepath.Join(t.TempDir(), "nope.md")}, false, ""},
		{"empty path", Input{}, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Decide(c.in, os.Stat)
			if d.Deny != c.deny {
				t.Fatalf("deny=%v want %v (%q)", d.Deny, c.deny, d.Message)
			}
			if c.deny && !strings.Contains(d.Message, c.hint) {
				t.Fatalf("message %q lacks %q", d.Message, c.hint)
			}
			if c.deny && !strings.Contains(d.Message, "KB") {
				t.Fatalf("message must state size in KB: %q", d.Message)
			}
		})
	}
}

func TestDecide_DirectoryIsAllowed(t *testing.T) {
	d := Decide(Input{FilePath: t.TempDir()}, os.Stat)
	if d.Deny {
		t.Fatalf("directory must fall open: %q", d.Message)
	}
}
