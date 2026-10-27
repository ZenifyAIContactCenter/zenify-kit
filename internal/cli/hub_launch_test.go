package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recTmux records every tmux call; calls named in failOn return an error.
type recTmux struct {
	calls  []string
	failOn map[string]bool
}

func (r *recTmux) Run(args ...string) ([]byte, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	if r.failOn[args[0]] {
		return nil, errors.New("fail " + args[0])
	}
	if args[0] == "-V" {
		return []byte("tmux 3.4\n"), nil
	}
	return nil, nil
}

func withHubEnv(t *testing.T, rt *recTmux, attached *[]string) {
	t.Helper()
	old := hubLauncher
	hubLauncher = hubEnv{
		lookPath: lookPathWith("tmux"),
		runner:   rt,
		self:     func() (string, error) { return "/Users/John Doe/it's/zenify", nil },
		attach: func(args ...string) error {
			*attached = args
			return nil
		},
	}
	t.Cleanup(func() { hubLauncher = old })
}

func hubWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".zenify"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".zenify", "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runHub(t *testing.T) (string, error) {
	t.Helper()
	cmd := newHubCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)
	err := cmd.Execute()
	return out.String(), err
}

func TestHubCmd_InsideTmux(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "x")
	rt := &recTmux{}
	var attached []string
	withHubEnv(t, rt, &attached)
	out, err := runHub(t)
	if err != nil {
		t.Fatalf("want exit 0, got %v", err)
	}
	if !strings.Contains(out, "already inside tmux") {
		t.Errorf("output %q missing hint", out)
	}
	if attached != nil {
		t.Errorf("must not attach, got %v", attached)
	}
}

func TestHubCmd_NewSession(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true}}
	var attached []string
	withHubEnv(t, rt, &attached)
	if _, err := runHub(t); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"new-session -d -s zenify -c " + root + " -n agent claude",
		`split-window -d -h -b -l 30% -t =zenify:agent -c ` + root + ` '/Users/John Doe/it'\''s/zenify' hub sidebar`,
		"set-option -w -t =zenify:agent allow-passthrough on",
	}
	for _, w := range want {
		found := false
		for _, c := range rt.calls {
			if c == w {
				found = true
			}
		}
		if !found {
			t.Errorf("calls missing %q in %q", w, rt.calls)
		}
	}
	if got := strings.Join(attached, " "); got != "attach -t =zenify" {
		t.Errorf("attach args = %q", got)
	}
}

func TestHubCmd_ExistingSessionOnlyAttaches(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{}
	var attached []string
	withHubEnv(t, rt, &attached)
	if _, err := runHub(t); err != nil {
		t.Fatal(err)
	}
	for _, c := range rt.calls {
		if strings.HasPrefix(c, "new-session") || strings.HasPrefix(c, "split-window") {
			t.Errorf("unexpected %q for existing session", c)
		}
	}
	if got := strings.Join(attached, " "); got != "attach -t =zenify" {
		t.Errorf("attach args = %q", got)
	}
}

func TestHubCmd_PassthroughFailureNotFatal(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	t.Setenv("HOME", t.TempDir())
	rt := &recTmux{failOn: map[string]bool{"has-session": true, "set-option": true}}
	var attached []string
	withHubEnv(t, rt, &attached)
	if _, err := runHub(t); err != nil {
		t.Fatalf("passthrough failure must not fail: %v", err)
	}
	if attached == nil {
		t.Error("must still attach")
	}
}

func TestHubCmd_NoWorkspace(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("TMUX", "")
	rt := &recTmux{}
	var attached []string
	withHubEnv(t, rt, &attached)
	_, err := runHub(t)
	if err == nil || !strings.Contains(err.Error(), "not inside a zenify workspace") {
		t.Fatalf("err = %v", err)
	}
}

func TestHubCmd_SplitFailureKillsSession(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true, "split-window": true}}
	var attached []string
	withHubEnv(t, rt, &attached)
	_, err := runHub(t)
	if err == nil || !strings.Contains(err.Error(), "setting up tmux session failed: fail split-window") {
		t.Fatalf("err = %v", err)
	}
	if last := rt.calls[len(rt.calls)-1]; last != "kill-session -t =zenify" {
		t.Errorf("last call = %q, want kill-session", last)
	}
	if attached != nil {
		t.Errorf("must not attach, got %v", attached)
	}
}

func TestHubCmd_KillFailureJoined(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true, "split-window": true, "kill-session": true}}
	var attached []string
	withHubEnv(t, rt, &attached)
	_, err := runHub(t)
	if err == nil || !strings.Contains(err.Error(), "fail kill-session") {
		t.Fatalf("kill error swallowed: %v", err)
	}
}

func TestHubCmd_AttachErrorWrapped(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{}
	var attached []string
	withHubEnv(t, rt, &attached)
	hubLauncher.attach = func(...string) error { return errors.New("boom") }
	_, err := runHub(t)
	if err == nil || !strings.Contains(err.Error(), "tmux attach: boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestHubCmd_NewSessionFailureKillsNothing(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true, "new-session": true}}
	var attached []string
	withHubEnv(t, rt, &attached)
	_, err := runHub(t)
	if err == nil || !strings.Contains(err.Error(), "fail new-session") {
		t.Fatalf("err = %v", err)
	}
	for _, c := range rt.calls {
		if strings.HasPrefix(c, "kill-session") {
			t.Errorf("killed a session it did not create: %q", rt.calls)
		}
	}
}
