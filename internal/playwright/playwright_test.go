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
	if err := os.WriteFile(filepath.Join(dir, "chromium-1234"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := Options{
		Runner: func(name string, args []string) error { return nil }, // get succeeds → registered
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
		Runner: func(name string, args []string) error { return os.ErrNotExist }, // get fails → absent
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

func TestBootstrapRegistersWhenAbsent(t *testing.T) {
	var calls [][]string
	getErr := os.ErrNotExist // get fails → absent
	o := Options{
		Runner: func(name string, args []string) error {
			calls = append(calls, append([]string{name}, args...))
			if len(args) >= 2 && args[0] == "mcp" && args[1] == "get" {
				return getErr
			}
			return nil
		},
		Getenv: func(string) string { return "" },
		GOOS:   "linux",
	}
	if err := Bootstrap(o); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// must include: claude mcp get, claude mcp add ... -s user, npx playwright install chromium
	var sawAdd, sawInstall bool
	for _, c := range calls {
		if c[0] == "claude" && len(c) >= 3 && c[1] == "mcp" && c[2] == "add" {
			sawAdd = true
			joined := strings.Join(c, " ")
			if !strings.Contains(joined, "-s user") || !strings.Contains(joined, "@playwright/mcp@latest") {
				t.Fatalf("add command wrong: %v", c)
			}
		}
		if c[0] == "npx" && strings.Contains(strings.Join(c, " "), "playwright install chromium") {
			sawInstall = true
		}
	}
	if !sawAdd || !sawInstall {
		t.Fatalf("missing steps: add=%v install=%v (%v)", sawAdd, sawInstall, calls)
	}
}

func TestBootstrapSkipsAddWhenPresent(t *testing.T) {
	var addCalled bool
	o := Options{
		Runner: func(name string, args []string) error {
			if name == "claude" && len(args) >= 2 && args[0] == "mcp" && args[1] == "add" {
				addCalled = true
			}
			return nil // every call (incl. get) succeeds → present
		},
		Getenv: func(string) string { return "" },
		GOOS:   "linux",
	}
	if err := Bootstrap(o); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if addCalled {
		t.Fatal("add must be skipped when the server is already registered")
	}
}

func TestBootstrapReturnsAddError(t *testing.T) {
	o := Options{
		Runner: func(name string, args []string) error {
			if name == "claude" && len(args) >= 2 && args[0] == "mcp" && args[1] == "get" {
				return os.ErrNotExist // absent → Bootstrap will attempt add
			}
			if name == "claude" && len(args) >= 2 && args[0] == "mcp" && args[1] == "add" {
				return errTest
			}
			return nil
		},
		Getenv: func(string) string { return "" },
		GOOS:   "linux",
	}
	if err := Bootstrap(o); err == nil {
		t.Fatal("expected the add error to propagate")
	}
}

var errTest = errors.New("boom")
