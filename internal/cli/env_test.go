package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

// fakeKeys is a manifest slice: A secret, B public.
func fakeKeys() []manifest.SecretKey {
	return []manifest.SecretKey{
		{Name: "A", Description: "key a", Secret: true},
		{Name: "B", Description: "url b", Secret: false},
	}
}

// envFixture builds deps over a temp workspace holding the given .env content.
func envFixture(t *testing.T, content string) (envDeps, string) {
	t.Helper()
	ws := t.TempDir()
	if content != "" {
		p := devenv.Path(ws)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := &manifest.Manifest{
		SecretKeys: fakeKeys(),
		Repos: []manifest.Repo{
			{Name: "lumi", SecretKeys: []manifest.SecretKey{{Name: "OPENROUTER_API_KEY", Secret: true}}},
		},
	}
	return envDeps{
		workspace: func() (string, bool) { return ws, true },
		manifest:  func(string) (*manifest.Manifest, error) { return m, nil },
		isTTY:     func() bool { return true },
		getenv:    func(string) string { return "" },
		environ:   func() []string { return nil },
		pick:      func([]string) (string, error) { return "", nil },
	}, ws
}

func runEnv(d envDeps, args ...string) (string, error) {
	cmd := newEnvCmdWith(d)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestEnvList_NoSecretValues(t *testing.T) {
	d, _ := envFixture(t, "A='topsecret1'\nB='https://x'\nC='localval9'\n")
	out, err := runEnv(d, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "https://x") {
		t.Errorf("public value B missing:\n%s", out)
	}
	for _, leak := range []string{"topsecret1", "localval9"} {
		if strings.Contains(out, leak) {
			t.Errorf("output leaks %q:\n%s", leak, out)
		}
	}
	var cLine string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "C ") {
			cLine = l
		}
	}
	if !strings.Contains(cLine, "local") || !strings.Contains(cLine, "set") {
		t.Errorf("line C = %q, want set + local", cLine)
	}
}

func TestEnvExec_MasksAndPassesExit(t *testing.T) {
	d, _ := envFixture(t, "A='abc#def12'\n")
	d.getenv = os.Getenv
	d.environ = os.Environ
	out, err := runEnv(d, "exec", "--", "sh", "-c", "echo $A; exit 3")
	if out != "***\n" {
		t.Errorf("stdout = %q, want %q", out, "***\n")
	}
	if got := exitcode.Code(err); got != 3 {
		t.Errorf("exit code = %d (err %v), want 3", got, err)
	}
}

func TestEnvExec_EnvWins(t *testing.T) {
	d, _ := envFixture(t, "B='fromfile'\n")
	d.getenv = func(k string) string {
		if k == "B" {
			return "fromenv"
		}
		return ""
	}
	d.environ = func() []string { return append(os.Environ(), "B=fromenv") }
	out, err := runEnv(d, "exec", "--", "sh", "-c", "echo $B")
	if err != nil {
		t.Fatal(err)
	}
	if out != "fromenv\n" {
		t.Errorf("stdout = %q, want fromenv", out)
	}
}

func TestEnvSet_RequiresTTY(t *testing.T) {
	d, _ := envFixture(t, "")
	d.isTTY = func() bool { return false }
	_, err := runEnv(d, "set", "A")
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Errorf("err = %v, want terminal error", err)
	}
}

func TestEnvSet_Desc(t *testing.T) {
	d, ws := envFixture(t, "")
	d.prompt = func(keys []string) (map[string]string, error) {
		return map[string]string{keys[0]: "val12345"}, nil
	}
	if _, err := runEnv(d, "set", "NEWKEY", "--desc", "x"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(devenv.Path(ws))
	if string(b) != "# x\nNEWKEY='val12345'\n" {
		t.Errorf("file = %q", b)
	}
}

func TestEnvSet_RejectsNewline(t *testing.T) {
	d, ws := envFixture(t, "")
	d.prompt = func(keys []string) (map[string]string, error) {
		return map[string]string{keys[0]: "ab\nINJECT=1"}, nil
	}
	_, err := runEnv(d, "set", "A")
	if err == nil || !strings.Contains(err.Error(), "xuống dòng") { //znf:allow-lang
		t.Errorf("err = %v, want newline refusal", err)
	}
	if _, serr := os.Stat(devenv.Path(ws)); serr == nil {
		t.Error("file written despite rejected value")
	}
}

func TestEnvForm_FirstRoundOnlyEmptyGlobals(t *testing.T) {
	d, ws := envFixture(t, "A='already'\n")
	var rounds [][]string
	d.prompt = func(keys []string) (map[string]string, error) {
		rounds = append(rounds, keys)
		return map[string]string{keys[0]: "newval123"}, nil
	}
	picks := []string{"OPENROUTER_API_KEY", ""}
	d.pick = func([]string) (string, error) {
		p := picks[0]
		picks = picks[1:]
		return p, nil
	}
	if _, err := runEnv(d); err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 2 {
		t.Fatalf("rounds = %v, want 2", rounds)
	}
	if strings.Join(rounds[0], ",") != "B" {
		t.Errorf("first round = %v, want only empty global B", rounds[0])
	}
	if strings.Join(rounds[1], ",") != "OPENROUTER_API_KEY" {
		t.Errorf("second round = %v", rounds[1])
	}
	f, _ := devenv.Load(devenv.Path(ws))
	if v, _ := f.Get("OPENROUTER_API_KEY"); v != "newval123" {
		t.Errorf("picked key not saved")
	}
}

func TestEnvForm_RejectsNewline(t *testing.T) {
	d, _ := envFixture(t, "")
	d.prompt = func(keys []string) (map[string]string, error) {
		return map[string]string{keys[0]: "a\rb"}, nil
	}
	_, err := runEnv(d)
	if err == nil || !strings.Contains(err.Error(), "xuống dòng") { //znf:allow-lang
		t.Errorf("err = %v, want newline refusal", err)
	}
}

func TestEnvEdit_CreatesFile0600(t *testing.T) {
	d, ws := envFixture(t, "")
	var opened string
	d.editor = func(path string) error { opened = path; return nil }
	if _, err := runEnv(d, "edit"); err != nil {
		t.Fatal(err)
	}
	if opened != devenv.Path(ws) {
		t.Errorf("editor opened %q", opened)
	}
	fi, err := os.Stat(opened)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("stat = %v, %v; want 0600", fi, err)
	}
}

func TestEnvNoWorkspace(t *testing.T) {
	d, _ := envFixture(t, "")
	d.workspace = func() (string, bool) { return "", false }
	_, err := runEnv(d, "list")
	if err == nil {
		t.Errorf("want error without workspace")
	}
}

func TestEnvHelp_MentionsPersonalNotAppEnv(t *testing.T) {
	cmd := newEnvCmd()
	if !strings.Contains(cmd.Short, "không phải env chạy app") { //znf:allow-lang
		t.Errorf("Short = %q", cmd.Short)
	}
}
