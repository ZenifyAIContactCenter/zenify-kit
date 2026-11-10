// internal/e2e/harness_test.go
package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHarness_MaterializesFixtures(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"fixtures.ts", "playwright.config.ts", "package.json", "auth.setup.ts"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s must exist: %v", f, err)
		}
	}
}

func TestHarness_AuthSetupAssertsLeftLogin(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "auth.setup.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "not.toHaveURL") {
		t.Fatal("auth.setup.ts must assert it left /login")
	}
}

// A failed login leaves error-context.md (an aria snapshot that prints input values)
// and, with tracing on, a trace that records fill() text: neither may carry the password.
func TestHarness_LoginFailureLeaksNoPassword(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatal(err)
	}
	setup, _ := os.ReadFile(filepath.Join(dir, "auth.setup.ts"))
	src := string(setup)
	// The snapshot is taken when the browser context closes, also after a timeout:
	// the page fixture's teardown (after use) empties the field before that close.
	use, clear, body := strings.Index(src, "await use(page);"), strings.Index(src, "set.call(el, '')"), strings.Index(src, "setup('authenticate'")
	if !strings.Contains(src, "base.extend(") || use < 0 || clear < use || clear > body {
		t.Error("auth.setup.ts must empty the password in the page fixture teardown")
	}
	// A failed fill() prints the value it was filling (measured on 1.64.1).
	if !strings.Contains(src, "(value withheld)") {
		t.Error("auth.setup.ts must rethrow a failed password fill without its value")
	}
	// process.env set in a test file reaches the journey workers too (measured on 1.64.1).
	if strings.Contains(src, "PLAYWRIGHT_NO_COPY_PROMPT") {
		t.Error("auth.setup.ts must not turn snapshots off by env: it leaks into journeys")
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "playwright.config.ts"))
	if !strings.Contains(string(cfg), `name: 'setup', testDir: '.', testMatch: /auth\.setup\.ts/, use: { trace: 'off' }`) {
		t.Error("the setup project must run with trace off")
	}
}

// A cold dev server compiles /login and the post-login route on first hit; the
// default 30s test timeout and 5s URL wait fail there (seen on a Vite app).
func TestHarness_LoginSurvivesColdDevServer(t *testing.T) {
	for _, p := range []string{"harness/auth.setup.ts", "../visual/harness/auth.setup.ts"} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, want := range []string{
			"setup.setTimeout(180_000)",
			"not.toHaveURL(/\\/login(\\?|$)/, { timeout: 90_000 })",
			"login rejected: HTTP",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("%s lacks %q", p, want)
			}
		}
	}
}
