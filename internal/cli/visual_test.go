package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func TestVisualCmd_Registered(t *testing.T) {
	root := NewRootCmd()
	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "visual" {
			found = true
		}
	}
	if !found {
		t.Fatal("`visual` command not registered on root")
	}
}

func TestVisualCheck_MismatchIsFail(t *testing.T) {
	// Runner giả lập docker exit≠0 (mismatch).
	repo := t.TempDir()
	snap := filepath.Join(repo, ".znf", "visual")
	if err := os.MkdirAll(snap, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, "routes.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := func(string, []string) error { return errors.New("exit status 1") }
	err := runVisualCheck(visualOpts{
		runner: runner, goos: "darwin", repo: repo, port: 3201, update: false,
		out: &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("mismatch phải trả error")
	}
	if exitcode.Code(err) != exitcode.Fail {
		t.Errorf("mismatch phải map Fail(1), got code %d", exitcode.Code(err))
	}
	if !strings.Contains(err.Error(), "__diff__") {
		t.Errorf("error phải trỏ ảnh diff, got %q", err.Error())
	}
}

func TestDockerPreflight(t *testing.T) {
	okLook := func(string) (string, error) { return "/usr/bin/docker", nil }
	// missing binary → BadArgs
	if err := dockerPreflight(func(string) (string, error) { return "", errors.New("not found") },
		func() error { return nil }); exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("missing docker → BadArgs, got %v", err)
	}
	// daemon down → BadArgs
	if err := dockerPreflight(okLook, func() error { return errors.New("cannot connect") }); exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("daemon down → BadArgs, got %v", err)
	}
	// all ok → nil
	if err := dockerPreflight(okLook, func() error { return nil }); err != nil {
		t.Errorf("docker ready → nil, got %v", err)
	}
}
