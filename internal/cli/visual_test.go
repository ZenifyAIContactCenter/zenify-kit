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
	// Runner simulates docker exit≠0 (mismatch).
	repo := t.TempDir()
	snap := filepath.Join(repo, ".znf", "visual")
	if err := os.MkdirAll(snap, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, "routes.json"), []byte(`[{"name":"a","path":"/a"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := func(string, []string) error { return errors.New("exit status 1") }
	err := runVisualCheck(visualOpts{
		runner: runner, goos: "darwin", repo: repo, port: 3201, update: false,
		out: &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("mismatch should return an error")
	}
	if exitcode.Code(err) != exitcode.Fail {
		t.Errorf("mismatch should map to Fail(1), got code %d", exitcode.Code(err))
	}
	if !strings.Contains(err.Error(), "__diff__") {
		t.Errorf("error should point at the diff image, got %q", err.Error())
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

func writeVisualRoutes(t *testing.T, body string) string {
	t.Helper()
	repo := t.TempDir()
	snap := filepath.Join(repo, ".znf", "visual")
	if err := os.MkdirAll(snap, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, "routes.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestRunVisualCheck_EmptyRoutes(t *testing.T) {
	repo := writeVisualRoutes(t, "[]")
	var out bytes.Buffer
	called := false
	runner := func(string, []string) error { called = true; return nil }
	if err := runVisualCheck(visualOpts{runner: runner, goos: "darwin", repo: repo, port: 1, out: &out}); err != nil {
		t.Fatal(err)
	}
	if called || !strings.Contains(out.String(), "0 routes") {
		t.Errorf("runner called=%v out=%q", called, out.String())
	}
}

func TestRunVisualCheck_InvalidRoutesIsBadArgs(t *testing.T) {
	repo := writeVisualRoutes(t, `[{"name":"a","path":"/a","area":"x"}]`)
	called := false
	runner := func(string, []string) error { called = true; return nil }
	err := runVisualCheck(visualOpts{runner: runner, goos: "darwin", repo: repo, port: 1, out: &bytes.Buffer{}})
	if exitcode.Code(err) != exitcode.BadArgs || called {
		t.Errorf("code=%d called=%v err=%v", exitcode.Code(err), called, err)
	}
}

func TestVisualAddRouteCmd(t *testing.T) {
	repo := t.TempDir()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"visual", "add-route", "--repo", repo, "--name", "a", "--path", "/a",
		"--mask", "x", "--mask", "y", "--aria", "#c"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(repo, ".znf", "visual", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"x"`, `"y"`, `"aria": "#c"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("routes.json missing %s: %s", want, b)
		}
	}
	for _, want := range []string{"added route a (1 routes)", "next: zenify visual check --repo " + repo,
		"then: git add -f .znf/visual/routes.json", "a.aria.yml"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q: %s", want, out.String())
		}
	}

	root = NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"visual", "add-route", "--repo", repo, "--name", "b"})
	if err := root.Execute(); err == nil {
		t.Error("missing --path must fail")
	}
}
