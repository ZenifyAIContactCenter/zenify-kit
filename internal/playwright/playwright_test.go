package playwright

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowsersDirRespectsEnvOverride(t *testing.T) {
	getenv := func(k string) string {
		if k == "PLAYWRIGHT_BROWSERS_PATH" {
			return "/custom/pw"
		}
		return "/home/u"
	}
	if got := browsersDir(getenv, "linux"); got != "/custom/pw" {
		t.Fatalf("override ignored: %q", got)
	}
}

func TestBrowsersDirPerOS(t *testing.T) {
	getenv := func(k string) string {
		switch k {
		case "HOME":
			return "/home/u"
		case "LOCALAPPDATA":
			return `C:\Users\u\AppData\Local`
		}
		return ""
	}
	cases := map[string]string{
		"darwin": "/home/u/Library/Caches/ms-playwright",
		"linux":  "/home/u/.cache/ms-playwright",
	}
	for goos, want := range cases {
		if got := browsersDir(getenv, goos); got != want {
			t.Fatalf("%s: got %q want %q", goos, got, want)
		}
	}
	if got := browsersDir(getenv, "windows"); got != `C:\Users\u\AppData\Local\ms-playwright` {
		t.Fatalf("windows: %q", got)
	}
}

func TestStatusRegisteredAndBrowsersPresent(t *testing.T) {
	dir := t.TempDir()
	// a non-empty browsers dir
	if err := os.WriteFile(filepath.Join(dir, "chromium-1234"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	o := Options{
		Output: getOutput(desiredOut("/h")),
		Home:   "/h",
		Getenv: func(k string) string {
			if k == "PLAYWRIGHT_BROWSERS_PATH" {
				return dir
			}
			return ""
		},
		GOOS: "linux",
	}
	reg, browsers, detail := Status(o)
	if !reg || !browsers {
		t.Fatalf("want registered+present, got reg=%v browsers=%v (%s)", reg, browsers, detail)
	}
}

func TestStatusUnregisteredEmptyBrowsers(t *testing.T) {
	dir := t.TempDir() // empty
	o := Options{
		Output: func(string, []string) ([]byte, error) { return nil, os.ErrNotExist }, // get fails → absent
		Getenv: func(k string) string {
			if k == "PLAYWRIGHT_BROWSERS_PATH" {
				return dir
			}
			return ""
		},
		GOOS: "linux",
	}
	reg, browsers, _ := Status(o)
	if reg || browsers {
		t.Fatalf("want unregistered+absent, got reg=%v browsers=%v", reg, browsers)
	}
}

func getOutput(out string) func(string, []string) ([]byte, error) {
	return func(string, []string) ([]byte, error) { return []byte(out), nil }
}

func mcpOut(args string) string {
	return "playwright:\n  Scope: User config (available in all your projects)\n  Status: x Failed to connect\n  Type: stdio\n  Command: npx\n" + args
}

func desiredOut(home string) string {
	return mcpOut("  Args: " + strings.Join(DesiredArgs(home), " ") + "\n")
}

func TestDesiredArgs(t *testing.T) {
	got := DesiredArgs("/h")
	want := []string{"@playwright/mcp@latest", "--caps=storage", "--output-dir", filepath.Join("/h", ".zenify", "playwright")}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDesiredArgsWindowsHome(t *testing.T) {
	got := DesiredArgs(`C:\Users\u`)
	if last := got[len(got)-1]; !strings.HasPrefix(last, `C:\Users\u`) || !strings.HasSuffix(last, "playwright") {
		t.Fatalf("output dir not built from home: %q", last)
	}
}

func TestBootstrapMigration(t *testing.T) {
	const home = "/h"
	cases := []struct {
		name      string
		out       string
		getErr    error
		wantCalls []string // prefixes of mutating calls, in order
		wantOut   string
	}{
		{"absent", "", os.ErrNotExist, []string{"claude mcp add playwright -s user -- npx @playwright/mcp@latest --caps=storage"}, ""},
		{"legacy", mcpOut("  Args: @playwright/mcp@latest\n"), nil, []string{"claude mcp remove playwright -s user", "claude mcp add playwright -s user -- npx @playwright/mcp@latest --caps=storage"}, ""},
		{"custom", mcpOut("  Args: @playwright/mcp@latest --headless\n"), nil, nil, "custom MCP registration kept"},
		{"desired", desiredOut(home), nil, nil, ""},
		{"no args line", mcpOut(""), nil, nil, "custom MCP registration kept"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var calls []string
			var buf strings.Builder
			o := Options{
				Runner: func(name string, args []string) error {
					calls = append(calls, name+" "+strings.Join(args, " "))
					return nil
				},
				Output: func(string, []string) ([]byte, error) { return []byte(c.out), c.getErr },
				Getenv: func(string) string { return "" },
				GOOS:   "linux",
				Home:   home,
				Stdout: &buf,
			}
			if err := Bootstrap(o); err != nil {
				t.Fatal(err)
			}
			var mut []string
			for _, cl := range calls {
				if strings.HasPrefix(cl, "claude mcp") {
					mut = append(mut, cl)
				}
			}
			if len(mut) != len(c.wantCalls) {
				t.Fatalf("mutating calls %v, want prefixes %v", mut, c.wantCalls)
			}
			for i, w := range c.wantCalls {
				if !strings.HasPrefix(mut[i], w) {
					t.Fatalf("call %d = %q, want prefix %q", i, mut[i], w)
				}
			}
			if !strings.Contains(buf.String(), c.wantOut) {
				t.Fatalf("stdout %q missing %q", buf.String(), c.wantOut)
			}
			if last := calls[len(calls)-1]; last != "npx playwright install chromium" {
				t.Fatalf("chromium install step missing: %v", calls)
			}
		})
	}
}

func TestRegistrationMissingArgs(t *testing.T) {
	st, missing := Registration(Options{Output: getOutput(mcpOut("  Args: @playwright/mcp@latest --headless\n")), Home: "/h"})
	if st != RegCustom || len(missing) != 3 || missing[0] != "--caps=storage" {
		t.Fatalf("got %v %v", st, missing)
	}
}

func TestBootstrapReturnsAddError(t *testing.T) {
	o := Options{
		Runner: func(name string, args []string) error {
			if name == "claude" && len(args) >= 2 && args[0] == "mcp" && args[1] == "add" {
				return errTest
			}
			return nil
		},
		Output: func(string, []string) ([]byte, error) { return nil, os.ErrNotExist },
		Getenv: func(string) string { return "" },
		GOOS:   "linux",
		Home:   "/h",
	}
	if err := Bootstrap(o); err == nil {
		t.Fatal("expected the add error to propagate")
	}
}

var errTest = errors.New("boom")
