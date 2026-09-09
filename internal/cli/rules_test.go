package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func TestRulesLint_FlagsAndExits(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "bad.go"), []byte("package a\n// tiếng Việt\n"), 0o644)
	cmd := newRulesCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"lint", d, "--include-go"})
	err := cmd.Execute()
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("want Fail, got code %d (err=%v)", exitcode.Code(err), err)
	}
	if !strings.Contains(out.String()+errb.String(), "bad.go:2") {
		t.Fatalf("want file:line in output, got %q / %q", out.String(), errb.String())
	}
}

func TestRulesLint_CleanExitsOK(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "ok.md"), []byte("all English here\n"), 0o644)
	cmd := newRulesCmd()
	cmd.SetArgs([]string{"lint", d})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("want OK/nil, got %v", err)
	}
}
