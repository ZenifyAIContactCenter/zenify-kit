package cli

import (
	"bytes"
	"testing"
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
