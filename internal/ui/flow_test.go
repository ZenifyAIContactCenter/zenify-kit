package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestFlow_Plain_GlyphsNoEscapes(t *testing.T) {
	SetNoColor(false)
	var b bytes.Buffer // non-TTY → plain
	u := New(&b)
	f := u.Flow("zenify up")
	f.Group(MarkerDone, "Workspace")
	f.Line(StatusOK, "contact-center-be", "OK")
	f.Blank()
	f.Group(MarkerActive, "Applying")
	f.Note("7/11")
	f.Close("onboarding complete")

	out := b.String()
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("plain flow must have no ANSI, got %q", out)
	}
	for _, want := range []string{"┌", "│", "◆", "◇", "└", "Workspace", "contact-center-be", "onboarding complete"} {
		if !strings.Contains(out, want) {
			t.Fatalf("plain flow missing %q: %q", want, out)
		}
	}
}

func TestFlow_Styled_EmitsAnsiAndGlyph(t *testing.T) {
	var b bytes.Buffer
	r := lipgloss.NewRenderer(&b, termenv.WithProfile(termenv.TrueColor))
	r.SetColorProfile(termenv.TrueColor)
	u := &Writer{w: &b, styled: true, r: r}

	f := u.Flow("zenify up")
	f.Group(MarkerDone, "Workspace")
	f.Line(StatusOK, "repo", "OK")
	f.Close("done")

	out := b.String()
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("styled flow must contain ANSI, got %q", out)
	}
	if !strings.Contains(out, "┌") || !strings.Contains(out, "│") {
		t.Fatalf("styled flow missing frame glyphs, got %q", out)
	}
}
