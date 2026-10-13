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
	want := []string{MCPPackage, "--isolated", "--caps=storage", "--output-dir", filepath.Join("/h", ".zenify", "playwright")}
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
	const spaced = "/Users/John Doe"
	addFor := func(h string) string {
		return "claude mcp add playwright -s user -- npx " + strings.Join(DesiredArgs(h), " ")
	}
	const rm = "claude mcp remove playwright -s user"
	cases := []struct {
		name      string
		home      string
		out       string
		getErr    error
		wantCalls []string // exact mutating calls, in order
		wantOut   string
	}{
		{"absent", home, "", os.ErrNotExist, []string{addFor(home)}, ""},
		{"legacy", home, mcpOut("  Args: @playwright/mcp@latest\n"), nil, []string{rm, addFor(home)}, ""},
		{"custom", home, mcpOut("  Args: @playwright/mcp@latest --headless\n"), nil, nil, "custom MCP registration kept"},
		{"desired", home, desiredOut(home), nil, nil, ""},
		{"no args line", home, mcpOut(""), nil, nil, "custom MCP registration kept"},
		{"desired with spaced home", spaced, desiredOut(spaced), nil, nil, ""},
		{"legacy at local scope", home, strings.Replace(mcpOut("  Args: @playwright/mcp@latest\n"), "User config (available in all your projects)", "Local config (private to you in this project)", 1), nil, nil, "custom MCP registration kept"},
		{"no scope line", home, strings.Replace(mcpOut("  Args: @playwright/mcp@latest\n"), "  Scope: User config (available in all your projects)\n", "", 1), nil, nil, "custom MCP registration kept"},
		{"legacy with spaced home", spaced, mcpOut("  Args: @playwright/mcp@latest\n"), nil, []string{rm, addFor(spaced)}, ""},
		{"older pin with kit flags", home, mcpOut("  Args: @playwright/mcp@0.0.1 " + strings.Join(DesiredArgs(home)[1:], " ") + "\n"), nil, []string{rm, addFor(home)}, ""},
		{"older pin with user flags", home, mcpOut("  Args: @playwright/mcp@0.0.1 --headless\n"), nil, nil, "custom MCP registration kept"},
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
				Home:   c.home,
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
			if strings.Join(mut, "\n") != strings.Join(c.wantCalls, "\n") {
				t.Fatalf("mutating calls %v, want %v", mut, c.wantCalls)
			}
			if !strings.Contains(buf.String(), c.wantOut) {
				t.Fatalf("stdout %q missing %q", buf.String(), c.wantOut)
			}
			if last := calls[len(calls)-1]; last != "npx --yes --package "+MCPPackage+" playwright install chromium" {
				t.Fatalf("chromium install step missing: %v", calls)
			}
		})
	}
}

func TestRegistrationMissingArgs(t *testing.T) {
	st, missing := Registration(Options{Output: getOutput(mcpOut("  Args: @playwright/mcp@latest --headless\n")), Home: "/h"})
	if st != RegCustom || len(missing) != 5 || missing[0] != MCPPackage || missing[1] != "--isolated" {
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

func codexFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "codex_mcp_get.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRegistrationCodexParsesFixture(t *testing.T) {
	fx := codexFixture(t)
	var gotName string
	var gotArgs []string
	o := Options{
		Target: TargetCodex,
		Output: func(name string, args []string) ([]byte, error) {
			gotName, gotArgs = name, args
			return fx, nil
		},
		Home: "/Users/believe",
	}
	st, _ := Registration(o)
	if st != RegDesired {
		t.Fatalf("fixture should be desired, got %v", st)
	}
	if gotName != "codex" || strings.Join(gotArgs, " ") != "mcp get playwright --json" {
		t.Fatalf("wrong command: %s %v", gotName, gotArgs)
	}
	o.Output = func(string, []string) ([]byte, error) {
		return []byte(`{"transport":{"command":"npx","args":["@playwright/mcp@latest"]}}`), nil
	}
	if st, _ := Registration(o); st != RegLegacyDefault {
		t.Fatalf("old args should be legacy-default, got %v", st)
	}
	o.Output = func(string, []string) ([]byte, error) {
		return []byte(`{"transport":{"command":"npx","args":["@playwright/mcp@0.0.1","--isolated","--caps=storage","--output-dir","/Users/believe/.zenify/playwright"]}}`), nil
	}
	if st, _ := Registration(o); st != RegLegacyDefault {
		t.Fatalf("older pin with kit flags should be legacy-default, got %v", st)
	}
	o.Output = func(string, []string) ([]byte, error) {
		return []byte(`{"transport":{"command":"npx","args":["@playwright/mcp@latest","--browser","firefox"]}}`), nil
	}
	if st, _ := Registration(o); st != RegCustom {
		t.Fatalf("user flags should be custom, got %v", st)
	}
	o.Home = "/other"
	o.Output = func(string, []string) ([]byte, error) { return fx, nil }
	if st, _ := Registration(o); st != RegCustom {
		t.Fatalf("other output-dir should be custom, got %v", st)
	}
	o.Output = func(string, []string) ([]byte, error) {
		return []byte(`{"transport":{"command":"node","args":["x.js"]}}`), nil
	}
	if st, _ := Registration(o); st != RegCustom {
		t.Fatalf("got %v, want custom", st)
	}
}

func TestRegistrationCodexAbsent(t *testing.T) {
	o := Options{
		Target: TargetCodex,
		Output: func(string, []string) ([]byte, error) { return nil, errTest },
		Home:   "/h",
	}
	if st, _ := Registration(o); st != RegAbsent {
		t.Fatalf("got %v, want absent", st)
	}
}

func TestBootstrapCodexAddsWithoutScopeFlag(t *testing.T) {
	var calls [][]string
	o := Options{
		Target: TargetCodex,
		Runner: func(name string, args []string) error {
			calls = append(calls, append([]string{name}, args...))
			return nil
		},
		Output: func(string, []string) ([]byte, error) { return nil, errTest },
		Getenv: func(string) string { return "" },
		GOOS:   "linux",
		Home:   "/h",
	}
	if err := Bootstrap(o); err != nil {
		t.Fatal(err)
	}
	want := append([]string{"codex", "mcp", "add", "playwright", "--", "npx"}, DesiredArgs("/h")...)
	if len(calls) != 1 || strings.Join(calls[0], "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("calls %v, want [%v]", calls, want)
	}
	for _, a := range calls[0] {
		if a == "-s" || a == "install" {
			t.Fatalf("unexpected %q in %v", a, calls[0])
		}
	}
}

func TestBootstrapCodexLegacyRemovesThenAdds(t *testing.T) {
	var calls [][]string
	o := Options{
		Target: TargetCodex,
		Runner: func(name string, args []string) error {
			calls = append(calls, append([]string{name}, args...))
			return nil
		},
		Output: func(string, []string) ([]byte, error) {
			return []byte(`{"transport":{"command":"npx","args":["@playwright/mcp@latest"]}}`), nil
		},
		Home: "/h",
	}
	if err := Bootstrap(o); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"codex", "mcp", "remove", "playwright"},
		append([]string{"codex", "mcp", "add", "playwright", "--", "npx"}, DesiredArgs("/h")...),
	}
	if len(calls) != 2 || strings.Join(calls[0], "\x00") != strings.Join(want[0], "\x00") || strings.Join(calls[1], "\x00") != strings.Join(want[1], "\x00") {
		t.Fatalf("calls %v, want %v", calls, want)
	}
}

func TestBootstrapSkipsInstalledChromium(t *testing.T) {
	install := "npx --yes --package " + MCPPackage + " playwright install chromium"
	cases := []struct {
		name        string
		markers     []string
		wantInstall bool
		wantOut     string
	}{
		{"both markers", []string{"chromium-1247", "chromium_headless_shell-1247"}, false, "chromium 1247 already installed"},
		{"headless shell missing", []string{"chromium-1247"}, true, "ensuring chromium"},
		{"other revision only", []string{"chromium-1248", "chromium_headless_shell-1248"}, true, "ensuring chromium"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, m := range c.markers {
				if err := os.MkdirAll(filepath.Join(dir, m), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, m, "INSTALLATION_COMPLETE"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var calls []string
			var buf strings.Builder
			o := Options{
				Runner: func(name string, args []string) error {
					calls = append(calls, name+" "+strings.Join(args, " "))
					return nil
				},
				Output: getOutput(desiredOut("/h")),
				Getenv: func(k string) string {
					if k == "PLAYWRIGHT_BROWSERS_PATH" {
						return dir
					}
					return ""
				},
				GOOS:   "linux",
				Home:   "/h",
				Stdout: &buf,
			}
			if err := Bootstrap(o); err != nil {
				t.Fatal(err)
			}
			got := strings.Contains(strings.Join(calls, "\n"), install)
			if got != c.wantInstall {
				t.Fatalf("install called = %v, want %v (calls %v)", got, c.wantInstall, calls)
			}
			if !strings.Contains(buf.String(), c.wantOut) {
				t.Fatalf("stdout %q missing %q", buf.String(), c.wantOut)
			}
		})
	}
}

// A bump of MCPVersion must come with the chromium revision its bundled
// playwright-core pins (browsers.json), or Bootstrap installs on every run.
func TestChromiumRevisionPinnedForMCPVersion(t *testing.T) {
	if chromiumRevision[MCPVersion] == "" {
		t.Fatalf("chromiumRevision has no entry for MCPVersion %s", MCPVersion)
	}
}
