package playwright

import (
	"encoding/json"
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

func TestRegistrationMissingArgs(t *testing.T) {
	st, missing := Registration(Options{Output: getOutput(mcpOut("  Args: @playwright/mcp@latest --headless\n")), Home: "/h"})
	if st != RegCustom || len(missing) != 5 || missing[0] != MCPPackage || missing[1] != "--isolated" {
		t.Fatalf("got %v %v", st, missing)
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

func envFor(dir string) func(string) string {
	return func(k string) string {
		if k == "PLAYWRIGHT_BROWSERS_PATH" {
			return dir
		}
		return ""
	}
}

func markChromium(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, m := range names {
		if err := os.MkdirAll(filepath.Join(dir, m), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, m, "INSTALLATION_COMPLETE"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func writeCLIPackage(t *testing.T, home, version string) {
	t.Helper()
	p := filepath.Join(CLIDir(home), "node_modules", "@playwright", "cli")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "package.json"), []byte(`{"name":"@playwright/cli","version":"`+version+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CLIScript(home), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// An interrupted npm install can leave package.json without the entry script.
func TestCLIInstalledRequiresScript(t *testing.T) {
	home := t.TempDir()
	writeCLIPackage(t, home, CLIVersion)
	if !CLIInstalled(home) {
		t.Fatal("full install reported missing")
	}
	if err := os.Remove(CLIScript(home)); err != nil {
		t.Fatal(err)
	}
	if CLIInstalled(home) {
		t.Error("package.json without playwright-cli.js reported installed")
	}
}

func recorder(calls *[]string) func(string, []string) error {
	return func(name string, args []string) error {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		return nil
	}
}

func TestInstallRunsNpmThenBrowser(t *testing.T) {
	home := t.TempDir()
	var calls []string
	o := Options{Runner: recorder(&calls), Home: home, Getenv: envFor(t.TempDir()), GOOS: "linux"}
	if err := Install(o); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"npm install --prefix " + home + "/.zenify/playwright/cli/pkg --no-audit --no-fund @playwright/cli@0.1.22",
		"node " + home + "/.zenify/playwright/cli/pkg/node_modules/@playwright/cli/playwright-cli.js install-browser chromium",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%v\nwant:\n%v", calls, want)
	}
}

func TestInstallSkipsWhenPresent(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	writeCLIPackage(t, home, "0.1.22")
	markChromium(t, dir, "chromium-1247", "chromium_headless_shell-1247")
	var calls []string
	var buf strings.Builder
	o := Options{Runner: recorder(&calls), Home: home, Getenv: envFor(dir), GOOS: "linux", Stdout: &buf}
	if err := Install(o); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatalf("want no commands, got %v", calls)
	}
	if !strings.Contains(buf.String(), "chromium 1247 already installed") {
		t.Fatalf("stdout %q", buf.String())
	}
}

func TestInstallReinstallsOtherVersion(t *testing.T) {
	home := t.TempDir()
	writeCLIPackage(t, home, "0.1.20")
	var calls []string
	o := Options{Runner: recorder(&calls), Home: home, Getenv: envFor(t.TempDir()), GOOS: "linux"}
	if err := Install(o); err != nil {
		t.Fatal(err)
	}
	if len(calls) == 0 || !strings.HasPrefix(calls[0], "npm install") {
		t.Fatalf("want npm install, got %v", calls)
	}
}

func TestDeregisterRemovesKitOwned(t *testing.T) {
	const legacy = "  Args: @playwright/mcp@latest\n"
	cases := []struct {
		name   string
		target Target
		out    string
		want   RegState
		rm     string
	}{
		{"claude desired", TargetClaude, desiredOut("/h"), RegDesired, "claude mcp remove playwright -s user"},
		{"claude legacy", TargetClaude, mcpOut(legacy), RegLegacyDefault, "claude mcp remove playwright -s user"},
		{"codex desired", TargetCodex, "", RegDesired, "codex mcp remove playwright"},
		{"codex legacy", TargetCodex, "", RegLegacyDefault, "codex mcp remove playwright"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var calls []string
			out := c.out
			if c.target == TargetCodex {
				args := DesiredArgs("/h")
				if c.want == RegLegacyDefault {
					args = []string{"@playwright/mcp@latest"}
				}
				b, _ := json.Marshal(map[string]any{"transport": map[string]any{"command": "npx", "args": args}})
				out = string(b)
			}
			o := Options{Target: c.target, Runner: recorder(&calls), Output: getOutput(out), Home: "/h"}
			got, err := Deregister(o)
			if err != nil || got != c.want {
				t.Fatalf("got %v, %v want %v", got, err, c.want)
			}
			if len(calls) != 1 || calls[0] != c.rm {
				t.Fatalf("calls %v want [%s]", calls, c.rm)
			}
		})
	}
}

func TestDeregisterKeepsCustom(t *testing.T) {
	var calls []string
	custom := mcpOut("  Args: some-other-server --flag\n")
	o := Options{Runner: recorder(&calls), Output: getOutput(custom), Home: "/h"}
	got, err := Deregister(o)
	if err != nil || got != RegCustom {
		t.Fatalf("got %v, %v", got, err)
	}
	if len(calls) != 0 {
		t.Fatalf("custom must not be removed: %v", calls)
	}
}

func TestDeregisterAbsent(t *testing.T) {
	var calls []string
	o := Options{Runner: recorder(&calls), Output: func(string, []string) ([]byte, error) { return nil, errTest }, Home: "/h"}
	got, err := Deregister(o)
	if err != nil || got != RegAbsent || len(calls) != 0 {
		t.Fatalf("got %v, %v, calls %v", got, err, calls)
	}
}

func TestStatusReadOnly(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	fail := func(string, []string) error { t.Fatal("runner must not be called"); return nil }
	o := Options{Runner: fail, Home: home, Getenv: envFor(dir), GOOS: "linux"}

	cli, br, detail := Status(o)
	if cli || br || !strings.Contains(detail, "zenify up") {
		t.Fatalf("missing: cli=%v br=%v detail=%q", cli, br, detail)
	}

	writeCLIPackage(t, home, CLIVersion)
	markChromium(t, dir, "chromium-1247", "chromium_headless_shell-1247")
	cli, br, detail = Status(o)
	if !cli || !br {
		t.Fatalf("want ok: cli=%v br=%v detail=%q", cli, br, detail)
	}
}

func TestChromiumRevisionPinnedForCLIVersion(t *testing.T) {
	if chromiumRevision[CLIVersion] != "1247" {
		t.Fatalf("chromiumRevision[%s] = %q", CLIVersion, chromiumRevision[CLIVersion])
	}
}
