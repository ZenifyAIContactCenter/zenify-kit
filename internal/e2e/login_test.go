package e2e

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

type call struct {
	dir, name string
	args, env []string
}

type fakeRun struct {
	calls   []call
	failOn  string // "test" fails the playwright test step
	content string
}

func (f *fakeRun) run(dir, name string, args []string, extraEnv []string) error {
	f.calls = append(f.calls, call{dir, name, args, extraEnv})
	if len(args) >= 2 && args[0] == "playwright" && args[1] == "test" {
		for _, e := range extraEnv {
			if p, ok := strings.CutPrefix(e, "STORAGE_STATE="); ok {
				_ = os.WriteFile(p, []byte(f.content), 0o644)
			}
		}
		if f.failOn == "test" {
			return errors.New("boom")
		}
	}
	return nil
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

var fullEnv = map[string]string{"E2E_DOMAIN": "d", "E2E_EMAIL": "e@x", "E2E_PASSWORD": "s3cret"}

func opts(home string, f *fakeRun, out *bytes.Buffer) LoginOptions {
	return LoginOptions{Home: home, BaseURL: "http://localhost:3327", Getenv: env(fullEnv),
		Run: f.run, Stdout: out, LockWait: time.Second}
}

func pwDir(home string) string { return filepath.Join(home, ".zenify", "playwright") }

func TestLogin_MissingEnvNamesOnly(t *testing.T) {
	f := &fakeRun{}
	o := opts(t.TempDir(), f, &bytes.Buffer{})
	o.Getenv = env(map[string]string{"E2E_DOMAIN": "d", "E2E_PASSWORD": "s3cret"})
	_, err := Login(o)
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("want BadArgs, got %v", err)
	}
	if !strings.Contains(err.Error(), "E2E_EMAIL") || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("bad message: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatal("Run must not be called")
	}
}

func TestLogin_NoSecretOnArgvOrEnv(t *testing.T) {
	f := &fakeRun{content: "x"}
	if _, err := Login(opts(t.TempDir(), f, &bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		all := strings.Join(append(append([]string{c.name}, c.args...), c.env...), " ")
		if strings.Contains(all, "s3cret") {
			t.Fatalf("secret leaked: %v", c)
		}
	}
	last := f.calls[len(f.calls)-1]
	if last.name != "npx" || strings.Join(last.args, " ") != "playwright test --project=setup" {
		t.Fatalf("last call = %v", last)
	}
	if !contains(last.env, "BASE_URL=http://localhost:3327") {
		t.Fatalf("env = %v", last.env)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestLogin_SuccessRenamesAndChmods(t *testing.T) {
	home := t.TempDir()
	f := &fakeRun{content: "STATEBODY"}
	var out bytes.Buffer
	p, err := Login(opts(home, f, &out))
	if err != nil {
		t.Fatal(err)
	}
	if p != StatePath(home) {
		t.Fatalf("path %s", p)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
	tmps, _ := filepath.Glob(filepath.Join(pwDir(home), "*.tmp"))
	if len(tmps) != 0 {
		t.Fatalf("tmp left: %v", tmps)
	}
	if !strings.Contains(out.String(), p) || strings.Contains(out.String(), "STATEBODY") {
		t.Fatalf("stdout %q", out.String())
	}
}

func TestLogin_FailureKeepsOldState(t *testing.T) {
	home := t.TempDir()
	_ = os.MkdirAll(pwDir(home), 0o755)
	_ = os.WriteFile(StatePath(home), []byte("old"), 0o600)
	f := &fakeRun{failOn: "test", content: "new"}
	_, err := Login(opts(home, f, &bytes.Buffer{}))
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("want Fail, got %v", err)
	}
	b, _ := os.ReadFile(StatePath(home))
	if string(b) != "old" {
		t.Fatalf("state changed: %s", b)
	}
	tmps, _ := filepath.Glob(filepath.Join(pwDir(home), "*.tmp"))
	if len(tmps) != 0 {
		t.Fatalf("tmp left: %v", tmps)
	}
}

func TestLogin_StaleLockIsBroken(t *testing.T) {
	home := t.TempDir()
	_ = os.MkdirAll(pwDir(home), 0o755)
	lock := filepath.Join(pwDir(home), "harness.lock")
	_ = os.WriteFile(lock, nil, 0o600)
	old := time.Now().Add(-11 * time.Minute)
	_ = os.Chtimes(lock, old, old)
	if _, err := Login(opts(home, &fakeRun{content: "x"}, &bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("lock must be released")
	}
}

func TestLogin_FreshLockTimesOut(t *testing.T) {
	home := t.TempDir()
	_ = os.MkdirAll(pwDir(home), 0o755)
	_ = os.WriteFile(filepath.Join(pwDir(home), "harness.lock"), nil, 0o600)
	f := &fakeRun{}
	_, err := Login(opts(home, f, &bytes.Buffer{}))
	if err == nil || !strings.Contains(err.Error(), "harness locked") {
		t.Fatalf("got %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatal("Run must not be called")
	}
}

func TestLogin_SkipsNpmInstallWhenPinned(t *testing.T) {
	home := t.TempDir()
	pkg := filepath.Join(pwDir(home), "harness", "node_modules", "@playwright", "test")
	_ = os.MkdirAll(pkg, 0o755)
	_ = os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"@playwright/test", "version": "1.55.0"}`), 0o644)
	f := &fakeRun{content: "x"}
	if _, err := Login(opts(home, f, &bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if c.name == "npm" {
			t.Fatalf("npm called: %v", c)
		}
	}
}

func TestLogin_InstallsWhenNotPinned(t *testing.T) {
	f := &fakeRun{content: "x"}
	if _, err := Login(opts(t.TempDir(), f, &bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
	if f.calls[0].name != "npm" {
		t.Fatalf("first call %v", f.calls[0])
	}
}

func TestLogin_RejectsNonLoopbackURL(t *testing.T) {
	for _, u := range []string{"https://attacker.example", "http://10.0.0.5:3000",
		"http://localhost.attacker.example", "ftp://localhost:1", "", "localhost:3327"} {
		f := &fakeRun{}
		o := opts(t.TempDir(), f, &bytes.Buffer{})
		o.BaseURL = u
		_, err := Login(o)
		if exitcode.Code(err) != exitcode.BadArgs {
			t.Fatalf("%q: want BadArgs, got %v", u, err)
		}
		if len(f.calls) != 0 {
			t.Fatalf("%q: Run must not be called", u)
		}
	}
}

func TestLogin_AcceptsLoopbackURL(t *testing.T) {
	for _, u := range []string{"http://localhost:3327", "http://127.0.0.1:3327",
		"http://[::1]:3327", "http://app.localhost:3327", "https://localhost:3327"} {
		f := &fakeRun{content: "x"}
		o := opts(t.TempDir(), f, &bytes.Buffer{})
		o.BaseURL = u
		if _, err := Login(o); err != nil {
			t.Fatalf("%q: %v", u, err)
		}
	}
}
