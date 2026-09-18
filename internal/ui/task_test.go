package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestFlowTask_Plain_PrintsResultNoEscapes(t *testing.T) {
	SetNoColor(false)
	var b bytes.Buffer // non-TTY → plain, spinner must not animate
	u := New(&b)
	f := u.Flow("zenify doctor")
	ran := false
	f.Task("git-auth", func() (Status, string) {
		ran = true
		return StatusOK, "authenticated"
	})
	f.Close("all checks passed")

	out := b.String()
	if !ran {
		t.Fatal("Task run func was not called")
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("plain Task must have no ANSI, got %q", out)
	}
	for _, want := range []string{"git-auth", "authenticated", "✓", "│"} {
		if !strings.Contains(out, want) {
			t.Fatalf("plain Task missing %q: %q", want, out)
		}
	}
}
