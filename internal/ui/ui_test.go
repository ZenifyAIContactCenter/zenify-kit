package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestPlainWriter_NoEscapes(t *testing.T) {
	SetNoColor(false)
	var b bytes.Buffer // not an *os.File → non-TTY → plain
	u := New(&b)
	if u.Styled() {
		t.Fatal("bytes.Buffer must be non-TTY → not styled")
	}
	u.Header("zenify up")
	u.Step(StatusOK, "Fetched origin", "12 repos")
	u.Note("dim helper")
	if strings.Contains(b.String(), "\x1b[") {
		t.Fatalf("plain output must have no ANSI, got %q", b.String())
	}
	for _, want := range []string{"▲", "zenify up", "✓", "Fetched origin", "12 repos", "dim helper"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("plain output missing %q: %q", want, b.String())
		}
	}
}

// TestStyledWriter_EmitsAnsiAndGlyph binds SC-1: styled mode must emit ANSI
// escapes alongside the glyph. New() gates styled on a real *os.File TTY, which
// a bytes.Buffer never is, so the styled Writer is constructed directly here
// (matching spinner_test.go's approach) with a renderer forced to TrueColor.
func TestStyledWriter_EmitsAnsiAndGlyph(t *testing.T) {
	var b bytes.Buffer
	r := lipgloss.NewRenderer(&b, termenv.WithProfile(termenv.TrueColor))
	r.SetColorProfile(termenv.TrueColor) // guard against auto-downgrade when the sink isn't a TTY
	u := &Writer{w: &b, styled: true, r: r}

	u.Header("zenify up")
	u.Step(StatusOK, "Fetched", "12 repos")

	out := b.String()
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("styled output must contain ANSI escapes, got %q", out)
	}
	if !strings.Contains(out, "▲") {
		t.Fatalf("styled output must contain the header glyph, got %q", out)
	}
}

func TestNoColorEnv_ForcesPlain(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	SetNoColor(false)
	var b bytes.Buffer
	u := New(&b)
	u.Step(StatusFail, "Build", "boom")
	if strings.Contains(b.String(), "\x1b[") {
		t.Fatalf("NO_COLOR must suppress ANSI, got %q", b.String())
	}
}

func TestSetNoColor_ForcesPlain(t *testing.T) {
	SetNoColor(true)
	t.Cleanup(func() { SetNoColor(false) })
	var b bytes.Buffer
	u := New(&b)
	u.Section("Repos")
	u.Table([]string{"NAME", "STATE"}, [][]string{{"web", "clone"}})
	if strings.Contains(b.String(), "\x1b[") {
		t.Fatalf("--no-color must suppress ANSI, got %q", b.String())
	}
	if !strings.Contains(b.String(), "web") || !strings.Contains(b.String(), "STATE") {
		t.Fatalf("table content missing: %q", b.String())
	}
}
