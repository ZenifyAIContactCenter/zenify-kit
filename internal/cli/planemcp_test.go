package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	kit "github.com/ZenifyAIContactCenter/zenify-kit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

const planeTestYAML = "org: o\nrepos:\n  - name: r\n    url: u\nplane:\n  url: https://plane.zenify.vn\n  workspace: zenify\n"

// planeWorkspace is a temp workspace whose .zenify/.env holds exactly envFile.
func planeWorkspace(t *testing.T, envFile string) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", ".env"), []byte(envFile), 0o600); err != nil {
		t.Fatal(err)
	}
	return ws
}

type planeRun struct {
	called bool
	name   string
	args   []string
	env    []string
}

// fakePlaneDeps stubs only the process boundaries; manifest is the test YAML.
func fakePlaneDeps(t *testing.T, ws string, rec *planeRun) planeMCPDeps {
	t.Helper()
	return planeMCPDeps{
		workspace: func() (string, bool) { return ws, ws != "" },
		manifest:  func() (*manifest.Manifest, error) { return manifest.Parse([]byte(planeTestYAML), "test") },
		getenv:    func(string) string { return "" },
		environ:   func() []string { return []string{"PATH=/bin", "PLANE_API_KEY=leaked", "HOME=/h"} },
		lookPath:  func(string) (string, error) { return "/usr/bin/uvx", nil },
		run: func(name string, args, env []string) error {
			rec.called, rec.name, rec.args, rec.env = true, name, args, env
			return nil
		},
	}
}

func execPlaneMCP(d planeMCPDeps) (stdout, stderr string, err error) {
	c := newPlaneMCPCmdWith(d)
	var out, errb bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errb)
	c.SetArgs(nil)
	err = c.Execute()
	return out.String(), errb.String(), err
}

func TestPlaneMCPEnv_ReplacesPlaneVars(t *testing.T) {
	got := planeMCPEnv([]string{"PATH=/bin", "PLANE_API_KEY=old", "PLANE_BASE_URL=https://evil"}, "https://plane.zenify.vn", "zenify", "pat")
	for _, want := range []string{"PATH=/bin", "PLANE_API_KEY=pat", "PLANE_BASE_URL=https://plane.zenify.vn", "PLANE_WORKSPACE_SLUG=zenify"} {
		if !slices.Contains(got, want) {
			t.Fatalf("env %v missing %q", got, want)
		}
	}
	n := 0
	for _, kv := range got {
		if strings.HasPrefix(kv, "PLANE_API_KEY=") {
			n++
		}
		if strings.Contains(kv, "old") || strings.Contains(kv, "evil") {
			t.Fatalf("env kept a stale value: %v", got)
		}
	}
	if n != 1 {
		t.Fatalf("want exactly one PLANE_API_KEY, got %d in %v", n, got)
	}
}

func TestPlaneMCP_RunsPinnedServer(t *testing.T) {
	ws := planeWorkspace(t, "PLANE_API_KEY=pat-123\nDB_URL=secret-db\n")
	var rec planeRun
	stdout, _, err := execPlaneMCP(fakePlaneDeps(t, ws, &rec))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if rec.name != "uvx" {
		t.Fatalf("name = %q", rec.name)
	}
	if want := []string{"--from", "plane-mcp-server==0.3.5", "plane-mcp-server", "stdio"}; !slices.Equal(rec.args, want) {
		t.Fatalf("args = %v, want %v", rec.args, want)
	}
	for _, want := range []string{"PATH=/bin", "HOME=/h", "PLANE_API_KEY=pat-123", "PLANE_BASE_URL=https://plane.zenify.vn", "PLANE_WORKSPACE_SLUG=zenify"} {
		if !slices.Contains(rec.env, want) {
			t.Fatalf("env %v missing %q", rec.env, want)
		}
	}
	for _, kv := range rec.env {
		if strings.Contains(kv, "secret-db") || strings.Contains(kv, "DB_URL") || strings.Contains(kv, "leaked") {
			t.Fatalf("env forwarded %q", kv)
		}
	}
}

func TestPlaneMCP_EnvPATWinsOverFile(t *testing.T) {
	ws := planeWorkspace(t, "PLANE_API_KEY=from-file\n")
	var rec planeRun
	d := fakePlaneDeps(t, ws, &rec)
	d.getenv = func(k string) string {
		if k == "PLANE_API_KEY" {
			return "from-env"
		}
		return ""
	}
	if _, _, err := execPlaneMCP(d); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(rec.env, "PLANE_API_KEY=from-env") {
		t.Fatalf("env %v: process env must win", rec.env)
	}
}

func TestPlaneMCP_NoPAT(t *testing.T) {
	ws := planeWorkspace(t, "DB_URL=secret-db\n")
	var rec planeRun
	stdout, stderr, err := execPlaneMCP(fakePlaneDeps(t, ws, &rec))
	if err == nil {
		t.Fatal("want an error")
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, want both empty (main prints the error to stderr)", stdout, stderr)
	}
	want := "zenify plane-mcp: chưa có PLANE_API_KEY. Tạo PAT ở https://plane.zenify.vn → Profile Settings → Personal Access Tokens, rồi chạy trong terminal: zenify env set PLANE_API_KEY" //znf:allow-lang
	if err.Error() != want {
		t.Fatalf("err = %q\nwant  %q", err, want)
	}
	if exitcode.Code(err) == 0 {
		t.Fatal("exit code must be non-zero")
	}
	if rec.called {
		t.Fatal("run must not be called")
	}
}

func TestPlaneMCP_NoUVX(t *testing.T) {
	ws := planeWorkspace(t, "PLANE_API_KEY=pat-123\n")
	var rec planeRun
	d := fakePlaneDeps(t, ws, &rec)
	d.lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	stdout, _, err := execPlaneMCP(d)
	if err == nil || !strings.Contains(err.Error(), "brew install uv") {
		t.Fatalf("err = %v, want the uv install hint", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if rec.called {
		t.Fatal("run must not be called")
	}
}

func TestPlaneMCP_NoWorkspace(t *testing.T) {
	var rec planeRun
	stdout, _, err := execPlaneMCP(fakePlaneDeps(t, "", &rec))
	if err == nil || err.Error() != "zenify plane-mcp: chưa có workspace, chạy zenify up trước" { //znf:allow-lang
		t.Fatalf("err = %v", err)
	}
	if exitcode.Code(err) == 0 || stdout != "" || rec.called {
		t.Fatalf("code=%d stdout=%q called=%v", exitcode.Code(err), stdout, rec.called)
	}
}

func TestPlaneMCP_NoPlaneInManifest(t *testing.T) {
	var rec planeRun
	d := fakePlaneDeps(t, planeWorkspace(t, "PLANE_API_KEY=pat-123\n"), &rec)
	d.manifest = func() (*manifest.Manifest, error) { return manifest.Parse([]byte(noBrokerYAML), "test") }
	stdout, _, err := execPlaneMCP(d)
	if err == nil || err.Error() != "zenify plane-mcp: manifest không khai báo plane" { //znf:allow-lang
		t.Fatalf("err = %v", err)
	}
	if exitcode.Code(err) != exitcode.BadArgs || stdout != "" || rec.called {
		t.Fatalf("code=%d stdout=%q called=%v", exitcode.Code(err), stdout, rec.called)
	}
}

func TestPlaneMCP_ChildExitCodePropagates(t *testing.T) {
	ws := planeWorkspace(t, "PLANE_API_KEY=pat-123\n")
	var rec planeRun
	d := fakePlaneDeps(t, ws, &rec)
	d.run = func(string, []string, []string) error { return exec.Command("sh", "-c", "exit 3").Run() }
	stdout, stderr, err := execPlaneMCP(d)
	if exitcode.Code(err) != 3 {
		t.Fatalf("code = %d (err %v), want 3", exitcode.Code(err), err)
	}
	if stdout != "" || strings.Contains(stderr+err.Error(), "pat-123") {
		t.Fatalf("stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}
}

// TestPlaneMCP_IgnoresCwdManifest resolves the manifest the production way:
// only workspace/run/lookPath are stubbed.
func TestPlaneMCP_IgnoresCwdManifest(t *testing.T) {
	saved := trustedBrokerManifest
	trustedBrokerManifest = func() ([]byte, string) { return kit.DefaultRepos, "embedded" }
	t.Cleanup(func() { trustedBrokerManifest = saved })
	t.Setenv("PLANE_API_KEY", "")

	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "manifest"), 0o700); err != nil {
		t.Fatal(err)
	}
	evil := "org: o\nrepos:\n  - name: r\n    url: u\nplane:\n  url: https://evil.example\n  workspace: evil\n"
	if err := os.WriteFile(filepath.Join(cwd, "manifest", "repos.yaml"), []byte(evil), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)

	ws := planeWorkspace(t, "PLANE_API_KEY=pat-123\n")
	var rec planeRun
	d := defaultPlaneMCPDeps()
	d.workspace = func() (string, bool) { return ws, true }
	d.lookPath = func(string) (string, error) { return "/usr/bin/uvx", nil }
	d.run = func(name string, args, env []string) error {
		rec.called, rec.env = true, env
		return nil
	}
	if _, _, err := execPlaneMCP(d); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(rec.env, "PLANE_BASE_URL=https://plane.zenify.vn") || !slices.Contains(rec.env, "PLANE_WORKSPACE_SLUG=zenify") {
		t.Fatalf("env %v: want the embedded Plane", rec.env)
	}
}

// TestPlaneMCP_RootStdoutClean runs the no-PAT branch through the real root.
func TestPlaneMCP_RootStdoutClean(t *testing.T) {
	saved := trustedBrokerManifest
	trustedBrokerManifest = func() ([]byte, string) { return kit.DefaultRepos, "embedded" }
	t.Cleanup(func() { trustedBrokerManifest = saved })
	t.Setenv("PLANE_API_KEY", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ZENIFY_HOME", t.TempDir())
	t.Chdir(planeWorkspace(t, "DB_URL=secret-db\n"))

	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"plane-mcp"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "zenify env set PLANE_API_KEY") {
		t.Fatalf("err = %v, want the no-PAT branch", err)
	}
	if out.String() != "" {
		t.Fatalf("root stdout = %q, want empty", out.String())
	}
}

// TestPlaneMCP_BinaryStderr checks the real binary: the message reaches
// stderr through main, stdout stays empty, the exit code is non-zero.
func TestPlaneMCP_BinaryStderr(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "zenify")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/zenify").CombinedOutput(); err != nil { //nolint:gosec // G204 -- fixed trusted binary build
		t.Fatalf("build: %v\n%s", err, out)
	}
	ws := planeWorkspace(t, "DB_URL=secret-db\n")
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PLANE_") {
			env = append(env, kv)
		}
	}
	c := exec.Command(bin, "plane-mcp") //nolint:gosec // G204 -- fixed trusted binary
	c.Dir = ws
	c.Env = append(env, "HOME="+t.TempDir(), "ZENIFY_HOME="+t.TempDir())
	c.Stdin = strings.NewReader("")
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	err := c.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() == 0 {
		t.Fatalf("want a non-zero exit, got %v", err)
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q", out.String())
	}
	if !strings.Contains(errb.String(), "zenify env set PLANE_API_KEY") || !strings.Contains(errb.String(), "https://plane.zenify.vn") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestPlaneMCP_PATNeverPrinted(t *testing.T) {
	cases := map[string]func(*planeMCPDeps){
		"no-uvx":     func(d *planeMCPDeps) { d.lookPath = func(string) (string, error) { return "", exec.ErrNotFound } },
		"no-plane":   func(d *planeMCPDeps) { d.manifest = func() (*manifest.Manifest, error) { return manifest.Parse([]byte(noBrokerYAML), "t") } },
		"child-exit": func(d *planeMCPDeps) { d.run = func(string, []string, []string) error { return exec.Command("sh", "-c", "exit 4").Run() } },
		"run-error":  func(d *planeMCPDeps) { d.run = func(string, []string, []string) error { return errors.New("spawn failed") } },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			var rec planeRun
			d := fakePlaneDeps(t, planeWorkspace(t, "PLANE_API_KEY=pat-123\n"), &rec)
			mut(&d)
			stdout, stderr, err := execPlaneMCP(d)
			if err == nil {
				t.Fatal("want an error")
			}
			if strings.Contains(stdout+stderr+err.Error(), "pat-123") {
				t.Fatalf("PAT printed: stdout=%q stderr=%q err=%v", stdout, stderr, err)
			}
		})
	}
}

func TestPlanePATSet(t *testing.T) {
	none := func(string) string { return "" }
	if planePATSet(none, planeWorkspace(t, "DB_URL=x\n")) {
		t.Fatal("no key → false")
	}
	if !planePATSet(none, planeWorkspace(t, "PLANE_API_KEY=pat-123\n")) {
		t.Fatal("key in file → true")
	}
	env := func(k string) string {
		if k == "PLANE_API_KEY" {
			return "e"
		}
		return ""
	}
	if !planePATSet(env, planeWorkspace(t, "")) {
		t.Fatal("key in env → true")
	}
}
