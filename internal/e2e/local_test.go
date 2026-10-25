package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
	if err := os.MkdirAll(filepath.Join(browsers, "chromium-1187"), 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", browsers)
	home, repo := t.TempDir(), localRepo(t)
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
