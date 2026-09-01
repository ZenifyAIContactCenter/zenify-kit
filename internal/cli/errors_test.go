package cli

import (
	"errors"
	"io"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func TestClassifyExecErr(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want int
	}{
		{"nil", nil, exitcode.OK},
		{"plain", errors.New("boom"), exitcode.Fail},
		{"already-coded", exitcode.New(exitcode.BadArgs, errors.New("x")), exitcode.BadArgs},
		{"unknown-command", errors.New(`unknown command "foo" for "zenify"`), exitcode.BadArgs},
		{"unknown-flag", errors.New("unknown flag: --nope"), exitcode.BadArgs},
	}
	for _, c := range cases {
		got := exitcode.Code(ClassifyExecErr(c.in))
		if got != c.want {
			t.Errorf("%s: code = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestRoot_UnknownCommand_ExitsBadArgs(t *testing.T) {
	root := NewRootCmd()
	root.SetArgs([]string{"definitely-not-a-command"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for unknown command")
	}
	if got := exitcode.Code(ClassifyExecErr(err)); got != exitcode.BadArgs {
		t.Errorf("code = %d, want %d", got, exitcode.BadArgs)
	}
}

func TestRoot_UnknownFlag_ExitsBadArgs(t *testing.T) {
	root := NewRootCmd()
	root.SetArgs([]string{"--definitely-not-a-flag"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for unknown flag")
	}
	if got := exitcode.Code(ClassifyExecErr(err)); got != exitcode.BadArgs {
		t.Errorf("code = %d, want %d", got, exitcode.BadArgs)
	}
}
