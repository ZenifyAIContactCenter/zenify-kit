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
	if !strings.Contains(cLine, "custom") || !strings.Contains(cLine, "set") || strings.Contains(cLine, "local") {
		t.Errorf("line C = %q, want set + custom, no local", cLine)
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

func TestEnvSet_DescRejectsNewline(t *testing.T) {
	d, ws := envFixture(t, "")
	d.prompt = func(keys []string) (map[string]string, error) {
		return map[string]string{keys[0]: "val12345"}, nil
	}
	_, err := runEnv(d, "set", "K", "--desc", "x\nEVIL=1")
	if err == nil || !strings.Contains(err.Error(), "xuống dòng") { //znf:allow-lang
		t.Errorf("err = %v, want newline refusal", err)
	}
	if _, serr := os.Stat(devenv.Path(ws)); serr == nil {
		t.Error("file written despite rejected --desc")
	}
}

func TestEnvList_CommentedOldValueNotPrinted(t *testing.T) {
	d, _ := envFixture(t, "# OLD_TOKEN=abc123secret\nNEW_KEY=x\n")
	out, err := runEnv(d, "list")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "abc123secret") || strings.Contains(out, "OLD_TOKEN") {
		t.Errorf("list prints a commented-out value:\n%s", out)
	}
}

// withMongoManifest swaps in a manifest holding MONGO_URL (staging) and MONGO_URL_PRD (production).
func withMongoManifest(d envDeps) envDeps {
	m := &manifest.Manifest{SecretKeys: []manifest.SecretKey{
		{Name: "MONGO_URL", Description: "mongo", Secret: true},
		{Name: "MONGO_URL_PRD", Description: "mongo prd", Secret: true, Group: manifest.GroupProduction},
	}}
	d.manifest = func(string) (*manifest.Manifest, error) { return m, nil }
	return d
}

func emptyPrompt(keys []string) (map[string]string, error) { return map[string]string{}, nil }

func readEnvFile(t *testing.T, ws string) string {
	t.Helper()
	b, err := os.ReadFile(devenv.Path(ws))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEnvSet_EmptyOnScaffoldedLineRejected(t *testing.T) {
	const content = "MONGO_URL=\n"
	d, ws := envFixture(t, content)
	d = withMongoManifest(d)
	d.prompt = emptyPrompt
	_, err := runEnv(d, "set", "MONGO_URL")
	if err == nil || !strings.Contains(err.Error(), "trống") { //znf:allow-lang
		t.Fatalf("err = %v, want empty-value refusal", err)
	}
	if got := readEnvFile(t, ws); got != content {
		t.Errorf("file changed: %q", got)
	}
}

func TestEnvSet_EmptyKeepsValueWithDesc(t *testing.T) {
	d, ws := envFixture(t, "MONGO_URL='x'\n")
	d = withMongoManifest(d)
	d.prompt = emptyPrompt
	if _, err := runEnv(d, "set", "MONGO_URL", "--desc", "d"); err != nil {
		t.Fatal(err)
	}
	f, _ := devenv.Load(devenv.Path(ws))
	if v, _ := f.Get("MONGO_URL"); v != "x" {
		t.Errorf("value = %q, want x kept", v)
	}
	if f.Desc("MONGO_URL") != "d" {
		t.Errorf("desc = %q, want d", f.Desc("MONGO_URL"))
	}
}

func TestEnvSet_CustomNeedsDesc(t *testing.T) {
	d, ws := envFixture(t, "")
	d = withMongoManifest(d)
	d.prompt = func(keys []string) (map[string]string, error) { return map[string]string{keys[0]: "val12345"}, nil }
	_, err := runEnv(d, "set", "FOO")
	if err == nil || !strings.Contains(err.Error(), "--desc") {
		t.Fatalf("err = %v, want --desc required", err)
	}
	if _, serr := os.Stat(devenv.Path(ws)); serr == nil {
		t.Error("file written despite missing --desc")
	}
	if _, err := runEnv(d, "set", "FOO", "--desc", "my foo"); err != nil {
		t.Fatal(err)
	}
	if got := readEnvFile(t, ws); got != "# my foo\nFOO='val12345'\n" {
		t.Errorf("file = %q", got)
	}
}

func TestEnvSet_CustomExistingDescNoFlagOK(t *testing.T) {
	d, ws := envFixture(t, "# described\nFOO='old'\n")
	d = withMongoManifest(d)
	d.prompt = func(keys []string) (map[string]string, error) { return map[string]string{keys[0]: "new12345"}, nil }
	if _, err := runEnv(d, "set", "FOO"); err != nil {
		t.Fatal(err)
	}
	if got := readEnvFile(t, ws); got != "# described\nFOO='new12345'\n" {
		t.Errorf("file = %q", got)
	}
}

func TestEnvList_CustomLabel(t *testing.T) {
	d, _ := envFixture(t, "# d\nFOO='v'\n")
	out, err := runEnv(d, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "FOO  set  custom") {
		t.Errorf("want custom label:\n%s", out)
	}
	if strings.Contains(out, "local") {
		t.Errorf("output still says local:\n%s", out)
	}
}

func TestEnvList_EmptyLineIsEmpty(t *testing.T) {
	d, _ := envFixture(t, "MONGO_URL=\n")
	d = withMongoManifest(d)
	out, err := runEnv(d, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "MONGO_URL  empty  manifest") {
		t.Errorf("want MONGO_URL empty:\n%s", out)
	}
}

func TestEnvForm_EmptyLineAsked(t *testing.T) {
	d, _ := envFixture(t, "MONGO_URL=\n")
	d = withMongoManifest(d)
	var rounds [][]string
	d.prompt = func(keys []string) (map[string]string, error) {
		rounds = append(rounds, keys)
		return map[string]string{keys[0]: "val12345"}, nil
	}
	if _, err := runEnv(d); err != nil {
		t.Fatal(err)
	}
	if len(rounds) == 0 || !contains(rounds[0], "MONGO_URL") {
		t.Errorf("rounds = %v, want first round to ask MONGO_URL", rounds)
	}
}

func TestEnvForm_FirstRoundSkipsProduction(t *testing.T) {
	d, _ := envFixture(t, "MONGO_URL=\nMONGO_URL_PRD=\n")
	d = withMongoManifest(d)
	var rounds [][]string
	d.prompt = func(keys []string) (map[string]string, error) {
		rounds = append(rounds, keys)
		return map[string]string{keys[0]: "val12345"}, nil
	}
	var picked []string
	d.pick = func(names []string) (string, error) { picked = names; return "", nil }
	if _, err := runEnv(d); err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 1 || strings.Join(rounds[0], ",") != "MONGO_URL" {
		t.Errorf("rounds = %v, want only MONGO_URL", rounds)
	}
	if !contains(picked, "MONGO_URL_PRD") {
		t.Errorf("pick list = %v, want MONGO_URL_PRD", picked)
	}
}
