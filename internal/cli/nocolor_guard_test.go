package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNoColorGuard_NoEscapesInStdout runs representative commands under NO_COLOR
// and asserts stdout carries no ANSI. Grow `cmds` as migration clusters land.
func TestNoColorGuard_NoEscapesInStdout(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	cmds := [][]string{
		{"doctor"},
		{"version"},
		{"analyze", "--help"},
		{"standards", "--help"},
		{"db-perf", "--help"},
		{"spec", "status", "--no-fetch"},
		{"wt", "--help"},
		{"migrate", "--help"},
		{"config", "--help"},
		{"docs", "--help"},
		{"skills", "--help"},
		{"review-log"},
		{"gate", "--help"},
		{"secret-scan", "--help"},
		{"update", "--help"},
	}
	for _, args := range cmds {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			cmd := NewRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(args)
			_ = cmd.Execute()
			if strings.Contains(out.String(), "\x1b[") {
				t.Fatalf("%v leaked ANSI to stdout under NO_COLOR: %q", args, out.String())
			}
		})
	}
}
