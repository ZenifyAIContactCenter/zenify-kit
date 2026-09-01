package playwright

import (
	"os"
	"path/filepath"
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
