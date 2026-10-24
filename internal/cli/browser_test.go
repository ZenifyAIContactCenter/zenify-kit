package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/playwright"
)

func browserHome(t *testing.T, installed bool) string {
	t.Helper()
	home := t.TempDir()
	if installed {
		dir := filepath.Join(playwright.CLIDir(home), "node_modules", "@playwright", "cli")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"version":"`+playwright.CLIVersion+`"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestBrowserSessionForms(t *testing.T) {
	home := browserHome(t, true)
	for _, args := range [][]string{
		{"-s=a", "open"}, {"-s", "a", "open"}, {"--session=a", "open"}, {"--session", "a", "open"},
	} {
		var got *exec.Cmd
		code := runBrowser(args, home, &bytes.Buffer{}, &bytes.Buffer{}, func(c *exec.Cmd) error { got = c; return nil })
		if code != 0 || got == nil {
			t.Fatalf("%v: code=%d cmd=%v", args, code, got)
		}
		wantDir := filepath.Join(playwright.SessionsDir(home), "a")
		if got.Dir != wantDir {
			t.Errorf("%v: Dir=%q want %q", args, got.Dir, wantDir)
		}
		want := []string{"node", playwright.CLIScript(home), "-s=a", "open"}
		if strings.Join(got.Args, "|") != strings.Join(want, "|") {
			t.Errorf("%v: argv=%v want %v", args, got.Args, want)
		}
	}
}

func TestBrowserRejects(t *testing.T) {
	home := browserHome(t, true)
	for _, args := range [][]string{
		{"open"}, {"-s=../x", "open"}, {"-s=", "open"},
		{"-s=a", "close-all"}, {"-s=a", "kill-all"}, {"-s=a", "install"},
	} {
		called := false
		var stderr bytes.Buffer
		code := runBrowser(args, home, &bytes.Buffer{}, &stderr, func(*exec.Cmd) error { called = true; return nil })
		if code != 2 || called {
			t.Errorf("%v: code=%d called=%v", args, code, called)
		}
		if strings.Count(strings.TrimSpace(stderr.String()), "\n") != 0 || strings.TrimSpace(stderr.String()) == "" {
			t.Errorf("%v: stderr want one line, got %q", args, stderr.String())
		}
	}
}

func TestBrowserSubcommandIsFirstPositional(t *testing.T) {
	home := browserHome(t, true)
	var got *exec.Cmd
	run := func(c *exec.Cmd) error { got = c; return nil }
	if code := runBrowser([]string{"-s=a", "eval", "close-all"}, home, &bytes.Buffer{}, &bytes.Buffer{}, run); code != 0 || got == nil {
		t.Fatalf("eval close-all: code=%d", code)
	}
	got = nil
	if code := runBrowser([]string{"-s", "x", "open"}, home, &bytes.Buffer{}, &bytes.Buffer{}, run); code != 0 || got == nil {
		t.Fatalf("-s x open: code=%d", code)
	}
	if !strings.HasSuffix(got.Dir, "/sessions/x") {
		t.Errorf("Dir=%q", got.Dir)
	}
}

func TestBrowserNotInstalled(t *testing.T) {
	home := browserHome(t, false)
	var stderr bytes.Buffer
	code := runBrowser([]string{"-s=a", "open"}, home, &bytes.Buffer{}, &stderr, func(*exec.Cmd) error { t.Fatal("run called"); return nil })
	if code != 1 || !strings.Contains(stderr.String(), "zenify up") {
		t.Errorf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestBrowserRewritesPaths(t *testing.T) {
	home := browserHome(t, true)
	sess := filepath.Join(playwright.SessionsDir(home), "a")
	var out bytes.Buffer
	run := func(c *exec.Cmd) error {
		_, _ = c.Stdout.Write([]byte("- [Screenshot](.playwright-cli/p.png)\npath: '.playwright-cli/p.png'\nsee /x/.playwright-cli/q.png\nlast .playwright-cli/z.png"))
		return nil
	}
	if code := runBrowser([]string{"-s=a", "screenshot"}, home, &out, &bytes.Buffer{}, run); code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "- [Screenshot](" + sess + "/.playwright-cli/p.png)\n" +
		"path: '" + sess + "/.playwright-cli/p.png'\n" +
		"see /x/.playwright-cli/q.png\n" +
		"last " + sess + "/.playwright-cli/z.png"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestBrowserEnv(t *testing.T) {
	t.Setenv("E2E_PASSWORD", "s")
	home := browserHome(t, true)
	var got *exec.Cmd
	runBrowser([]string{"-s=a", "open"}, home, &bytes.Buffer{}, &bytes.Buffer{}, func(c *exec.Cmd) error { got = c; return nil })
	has := map[string]bool{}
	for _, kv := range got.Env {
		has[kv] = true
		if strings.HasPrefix(kv, "E2E_PASSWORD=") {
			t.Errorf("env leaks %q", kv)
		}
	}
	if !has["NO_UPDATE_NOTIFIER=1"] {
		t.Error("NO_UPDATE_NOTIFIER=1 missing")
	}
}

func TestBrowserPassesExitCode(t *testing.T) {
	home := browserHome(t, true)
	err := exec.Command("sh", "-c", "exit 3").Run()
	code := runBrowser([]string{"-s=a", "open"}, home, &bytes.Buffer{}, &bytes.Buffer{}, func(*exec.Cmd) error { return err })
	if code != 3 {
		t.Errorf("code=%d want 3", code)
	}
}
