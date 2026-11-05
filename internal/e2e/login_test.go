package e2e

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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

// A lock file left by a killed run (or an older binary) is just a file: only a live
// OS lock blocks, so the next run proceeds at once.
func TestLogin_LeftoverLockFileIsNotALock(t *testing.T) {
	home := t.TempDir()
	_ = os.MkdirAll(pwDir(home), 0o755)
	lock := filepath.Join(pwDir(home), "harness.lock")
	_ = os.WriteFile(lock, []byte("12345"), 0o600)
	if _, err := Login(opts(home, &fakeRun{content: "x"}, &bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
	if isLocked(t, lock) {
		t.Fatal("lock must be released")
	}
}

func TestLogin_FreshLockTimesOut(t *testing.T) {
	home := t.TempDir()
	_ = os.MkdirAll(pwDir(home), 0o755)
	unlock, err := lockHarness(pwDir(home), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	f := &fakeRun{}
	_, err = Login(opts(home, f, &bytes.Buffer{}))
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
	_ = os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"@playwright/test", "version": "1.64.1"}`), 0o644)
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

func TestStatePathRole(t *testing.T) {
	home := "/h"
	if got, want := StatePathRole(home, ""), filepath.Join(home, ".zenify", "playwright", "state.json"); got != want {
		t.Errorf("no role = %q, want %q", got, want)
	}
	if got, want := StatePathRole(home, "agent"), filepath.Join(home, ".zenify", "playwright", "state-agent.json"); got != want {
		t.Errorf("agent = %q, want %q", got, want)
	}
	if StatePath(home) != StatePathRole(home, "") {
		t.Errorf("StatePath must equal StatePathRole(home, \"\")")
	}
}

func TestLogin_RejectsUnknownRole(t *testing.T) {
	_, err := Login(LoginOptions{Home: t.TempDir(), BaseURL: "http://localhost:1", Role: "../x",
		Getenv: func(string) string { return "v" }})
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("want BadArgs, got %v", err)
	}
}

// The setup project runs in the shared harness, so Login must hold harness.lock while
// Playwright runs (a concurrent `e2e run --local` would rewrite the harness under it).
func TestLogin_HoldsLockDuringSetup(t *testing.T) {
	for _, fail := range []bool{false, true} {
		home := t.TempDir()
		lock := filepath.Join(pwDir(home), "harness.lock")
		held := false
		f := &fakeRun{content: "{}"}
		if fail {
			f.failOn = "test"
		}
		o := opts(home, f, &bytes.Buffer{})
		inner := o.Run
		o.Run = func(dir, name string, args, extraEnv []string) error {
			if len(args) >= 2 && args[1] == "test" {
				held = isLocked(t, lock)
			}
			return inner(dir, name, args, extraEnv)
		}
		if _, err := Login(o); (err != nil) != fail {
			t.Fatalf("fail=%v: err = %v", fail, err)
		}
		if !held {
			t.Errorf("fail=%v: harness.lock not held while the setup project ran", fail)
		}
		if isLocked(t, lock) {
			t.Errorf("fail=%v: lock left held", fail)
		}
	}
}

func TestLogin_LockRecordsHolderPID(t *testing.T) {
	home := t.TempDir()
	lock := filepath.Join(pwDir(home), "harness.lock")
	var got string
	o := opts(home, &fakeRun{content: "x"}, &bytes.Buffer{})
	o.Run = func(dir, name string, args []string, extraEnv []string) error {
		b, _ := os.ReadFile(lock)
		got = string(b)
		return (&fakeRun{content: "x"}).run(dir, name, args, extraEnv)
	}
	if _, err := Login(o); err != nil {
		t.Fatal(err)
	}
	if got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("lock must name the holder's PID (for humans), got %q", got)
	}
}

// zenify killed mid-run (no unlock) while npm/playwright keeps going: the child that
// inherited the lock still holds it, so a new run waits; once the child is gone the
// OS releases it.
func TestLock_HeldByChildAfterHolderDies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows children cannot inherit the lock handle")
	}
	root := t.TempDir()
	lock := filepath.Join(root, "harness.lock")
	if _, err := lockHarness(root, time.Second); err != nil {
		t.Fatal(err)
	}
	child := exec.Command("sleep", "30")
	InheritLock(child)
	if err := child.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	defer func() { _ = child.Process.Kill(); _, _ = child.Process.Wait() }()
	dieWithoutUnlock() // the zenify process "is killed": its fd closes, no unlock
	if !isLocked(t, lock) {
		t.Fatal("the child inherited the lock: it must still be held")
	}
	_ = child.Process.Kill()
	_, _ = child.Process.Wait()
	if isLocked(t, lock) {
		t.Fatal("all holders gone: the OS must have released the lock")
	}
}

func TestInheritLock_NothingHeld(t *testing.T) {
	c := exec.Command("true")
	InheritLock(c)
	if len(c.ExtraFiles) != 0 {
		t.Fatalf("no lock held: ExtraFiles = %v", c.ExtraFiles)
	}
}

// isLocked reports whether some open file holds the OS lock on path.
func isLocked(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	busy, err := tryLockFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if !busy {
		_ = unlockFile(f)
	}
	return busy
}

// dieWithoutUnlock closes this process's lock fds the way process death does: no
// explicit unlock, so a child sharing the open file keeps the lock.
func dieWithoutUnlock() {
	heldMu.Lock()
	defer heldMu.Unlock()
	for _, f := range heldLocks {
		_ = f.Close()
	}
	heldLocks = nil
}

// An older zenify holds the harness with an empty O_EXCL file and no OS lock: a new
// binary must wait for it like the old ones do, not take the harness alongside it.
func TestLogin_LegacyHolderLockWaits(t *testing.T) {
	home := t.TempDir()
	_ = os.MkdirAll(pwDir(home), 0o755)
	_ = os.WriteFile(filepath.Join(pwDir(home), "harness.lock"), nil, 0o600)
	f := &fakeRun{}
	_, err := Login(opts(home, f, &bytes.Buffer{}))
	if err == nil || !strings.Contains(err.Error(), "harness locked") {
		t.Fatalf("an older binary's fresh lock must be waited for, got %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatal("Run must not be called")
	}
}

// The old rule for an abandoned empty lock still applies: past staleLockAge it is free.
func TestLogin_LegacyLeftoverPastStaleAgeIsFree(t *testing.T) {
	home := t.TempDir()
	_ = os.MkdirAll(pwDir(home), 0o755)
	lock := filepath.Join(pwDir(home), "harness.lock")
	_ = os.WriteFile(lock, nil, 0o600)
	old := time.Now().Add(-staleLockAge - time.Minute)
	_ = os.Chtimes(lock, old, old)
	if _, err := Login(opts(home, &fakeRun{content: "x"}, &bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
}

// Unlock removes the file like older binaries expect (they wait while it exists).
func TestLockHarness_UnlockRemovesFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows keeps the file: a deleted open file blocks re-creating it")
	}
	root := t.TempDir()
	unlock, err := lockHarness(root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if _, err := os.Stat(filepath.Join(root, "harness.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock file must be removed on unlock: %v", err)
	}
}

// Removing the file on unlock must not let two holders in. B opens the file; before B
// locks it, holder A unlocks (unlinking it) and C locks a new file at the path. B's
// lock on the unlinked file must not count: B waits while C holds the path.
func TestLockHarness_UnlinkedFileIsNotTheLock(t *testing.T) {
	if !removeOnUnlock {
		t.Skip("the lock file is never unlinked here")
	}
	root := t.TempDir()
	unlockA, err := lockHarness(root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var unlockC func()
	afterLockFileOpen = func() {
		afterLockFileOpen = nil
		unlockA()
		if unlockC, err = lockHarness(root, time.Second); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { afterLockFileOpen = nil })
	_, berr := lockHarness(root, time.Second)
	if unlockC != nil {
		defer unlockC()
	}
	if berr == nil || !strings.Contains(berr.Error(), "harness locked") {
		t.Fatalf("B must not hold the harness while C does, got %v", berr)
	}
}
