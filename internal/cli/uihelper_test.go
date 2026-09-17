package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
)

func TestUIOut_BindsCommandWriter_Plain(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	u := uiOut(cmd)
	u.Step(ui.StatusOK, "hello", "")
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("buffer writer must be plain: %q", out.String())
	}
	if !strings.Contains(out.String(), "hello") {
		t.Fatalf("missing content: %q", out.String())
	}
}
