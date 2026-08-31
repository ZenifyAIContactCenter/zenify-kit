package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

func TestRootCmd_HelpMentionsName(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("zenify")) {
		t.Errorf("help output missing 'zenify': %q", out.String())
	}
}

func TestRootCmd_VersionFlag(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--version: %v", err)
	}
	if !strings.Contains(out.String(), version.Current()) {
		t.Errorf("--version output %q does not contain %q", out.String(), version.Current())
	}
}
