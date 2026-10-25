package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type localCall struct {
	dir, name string
	args, env []string
}

func localRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	d := filepath.Join(repo, ".znf", "e2e")
	if err := os.MkdirAll(d, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "a.spec.ts"), []byte("// journey A"), 0o600); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestRunLocalArgs(t *testing.T) {
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", t.TempDir())
	home, repo := t.TempDir(), localRepo(t)
	harness := filepath.Join(playwrightDir(home), "harness")
	stale := filepath.Join(harness, ".znf", "e2e", "old.spec.ts")
	if err := os.MkdirAll(filepath.Dir(stale), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls []localCall
	err := RunLocal(LocalOptions{RepoDir: repo, BaseURL: "http://127.0.0.1:4100", Home: home,
		Env: []string{"E2E_EMAIL=a@b.c"},
		Run: func(dir, name string, args, env []string) error {
			calls = append(calls, localCall{dir, name, args, env})
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range calls {
		if c.name == "docker" {
			t.Fatalf("docker called: %+v", c)
		}
	}
	last := calls[len(calls)-1]
	if last.dir != harness || last.name != "npx" || strings.Join(last.args, " ") != "playwright test" {
		t.Fatalf("last call = %+v", last)
	}
	if !slices.Contains(last.env, "BASE_URL=http://127.0.0.1:4100") || !slices.Contains(last.env, "E2E_EMAIL=a@b.c") {
		t.Errorf("env = %v", last.env)
	}
	if !slices.ContainsFunc(last.env, func(s string) bool { return strings.HasPrefix(s, "STORAGE_STATE=") }) {
		t.Errorf("no STORAGE_STATE in %v", last.env)
	}
	got, err := os.ReadFile(filepath.Join(harness, ".znf", "e2e", "a.spec.ts"))
	if err != nil || string(got) != "// journey A" {
		t.Errorf("copied journey = %q, %v", got, err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file survived: %v", err)
	}
}

func TestRunLocalSkipsInstall(t *testing.T) {
	browsers := t.TempDir()
	for _, d := range []string{"chromium-1187", "chromium_headless_shell-1187"} {
		mkBrowser(t, filepath.Join(browsers, d), true)
	}
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", browsers)
	home, repo := t.TempDir(), localRepo(t)
	writeBrowsersJSON(t, home, "1187")
	pkg := filepath.Join(playwrightDir(home), "harness", "node_modules", "@playwright", "test")
	if err := os.MkdirAll(pkg, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"version":"1.55.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls []string
	err := RunLocal(LocalOptions{RepoDir: repo, BaseURL: "http://localhost:4100", Home: home,
		Run: func(_, name string, args, _ []string) error {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "npx playwright test" {
		t.Fatalf("calls = %v, want only the test run", calls)
	}
}

func TestRunLocalRejectsRemote(t *testing.T) {
	err := RunLocal(LocalOptions{RepoDir: localRepo(t), BaseURL: "https://staging.zenify.vn", Home: t.TempDir(),
		Run: func(_, _ string, _, _ []string) error { t.Fatal("ran"); return nil }})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("err = %v", err)
	}
}

func TestPrepareLocalDoesNotRunTests(t *testing.T) {
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", t.TempDir())
	var calls []string
	err := PrepareLocal(LocalOptions{Home: t.TempDir(),
		Run: func(_, name string, args, _ []string) error {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"npm install --no-audit --no-fund --silent", "npx playwright install chromium"}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func writeBrowsersJSON(t *testing.T, home, rev string) {
	t.Helper()
	d := filepath.Join(playwrightDir(home), "harness", "node_modules", "playwright-core")
	if err := os.MkdirAll(d, 0o750); err != nil {
		t.Fatal(err)
	}
	body := `{"browsers":[{"name":"firefox","revision":"9"},{"name":"chromium","revision":"` + rev + `"},` +
		`{"name":"chromium-headless-shell","revision":"` + rev + `"}]}`
	if err := os.WriteFile(filepath.Join(d, "browsers.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// mkBrowser creates a browser revision dir; complete adds the marker the installer writes last.
func mkBrowser(t *testing.T, dir string, complete bool) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if complete {
		if err := os.WriteFile(filepath.Join(dir, "INSTALLATION_COMPLETE"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func prepareWithBrowsers(t *testing.T, have, want string, writeJSON bool, dirs ...string) []string {
	t.Helper()
	return prepareWithCache(t, have, want, writeJSON, true, dirs...)
}

func prepareWithCache(t *testing.T, have, want string, writeJSON, complete bool, dirs ...string) []string {
	t.Helper()
	browsers := t.TempDir()
	if len(dirs) == 0 {
		dirs = []string{"chromium-", "chromium_headless_shell-"}
	}
	for _, d := range dirs {
		mkBrowser(t, filepath.Join(browsers, d+have), complete)
	}
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", browsers)
	home := t.TempDir()
	if writeJSON {
		writeBrowsersJSON(t, home, want)
	}
	pkg := filepath.Join(playwrightDir(home), "harness", "node_modules", "@playwright", "test")
	if err := os.MkdirAll(pkg, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"version":"1.55.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls []string
	if err := PrepareLocal(LocalOptions{Home: home, Run: func(_, name string, args, _ []string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	return calls
}

func TestPrepareChromiumRevision(t *testing.T) {
	install := "npx playwright install chromium"
	if c := prepareWithBrowsers(t, "1247", "1187", true); !slices.Equal(c, []string{install}) {
		t.Errorf("revision mismatch: calls = %v, want the install", c)
	}
	if c := prepareWithBrowsers(t, "1187", "1187", true); len(c) != 0 {
		t.Errorf("revision match: calls = %v, want none", c)
	}
	if c := prepareWithCache(t, "1187", "1187", true, false); !slices.Equal(c, []string{install}) {
		t.Errorf("half-installed cache (no INSTALLATION_COMPLETE): calls = %v, want the install", c)
	}
	if c := prepareWithBrowsers(t, "1187", "", false); !slices.Equal(c, []string{install}) {
		t.Errorf("no browsers.json: calls = %v, want the install", c)
	}
	// headless runs use chromium-headless-shell; chromium alone is not enough
	if c := prepareWithBrowsers(t, "1187", "1187", true, "chromium-"); !slices.Equal(c, []string{install}) {
		t.Errorf("no headless shell: calls = %v, want the install", c)
	}
}

func TestRunLocalSkipsOutputDir(t *testing.T) {
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", t.TempDir())
	home, repo := t.TempDir(), localRepo(t)
	out := filepath.Join(repo, ".znf", "e2e", "__output__")
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "trace.zip"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := RunLocal(LocalOptions{RepoDir: repo, BaseURL: "http://localhost:4100", Home: home,
		Run: func(_, _ string, _, _ []string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(playwrightDir(home), "harness", ".znf", "e2e", "__output__")); !os.IsNotExist(err) {
		t.Errorf("__output__ was copied: %v", err)
	}
}

// RunLocal swaps the shared harness's journeys, so it must hold harness.lock while it
// copies them and while Playwright runs, and release it afterwards (also on failure).
func TestRunLocalHoldsLockDuringRun(t *testing.T) {
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", t.TempDir())
	for _, fail := range []bool{false, true} {
		home, repo := t.TempDir(), localRepo(t)
		lock := filepath.Join(playwrightDir(home), "harness.lock")
		held := false
		err := RunLocal(LocalOptions{RepoDir: repo, BaseURL: "http://localhost:4100", Home: home, LockWait: time.Second,
			Run: func(_, name string, args, _ []string) error {
				if name+" "+strings.Join(args, " ") == "npx playwright test" {
					_, serr := os.Stat(lock)
					held = serr == nil
					if fail {
						return errors.New("red")
					}
				}
				return nil
			}})
		if (err != nil) != fail {
			t.Fatalf("fail=%v: err = %v", fail, err)
		}
		if !held {
			t.Errorf("fail=%v: harness.lock not held while playwright ran", fail)
		}
		if _, serr := os.Stat(lock); !os.IsNotExist(serr) {
			t.Errorf("fail=%v: lock left behind: %v", fail, serr)
		}
	}
}
