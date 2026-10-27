package cli

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

type fakeTmuxRunner struct {
	out string
	err error
}

func (f fakeTmuxRunner) Run(args ...string) ([]byte, error) { return []byte(f.out), f.err }

func lookPathWith(present ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, p := range present {
			if p == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestTmuxCheck_Missing(t *testing.T) {
	env := tmuxEnv{goos: "linux", lookPath: lookPathWith(), runner: fakeTmuxRunner{}}
	ok, detail := tmuxCheck(env).Run()
	if ok {
		t.Fatal("expected not ok when tmux is missing")
	}
	for _, want := range []string{"brew install tmux", "zenify doctor --fix"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q missing %q", detail, want)
		}
	}
}

func TestTmuxInstallCommand(t *testing.T) {
	cases := []struct {
		name     string
		env      tmuxEnv
		wantName string
		wantArgs []string
		wantOK   bool
	}{
		{"linux apt", tmuxEnv{goos: "linux", lookPath: lookPathWith("apt-get", "dnf")}, "sudo", []string{"apt-get", "install", "-y", "tmux"}, true},
		{"linux dnf", tmuxEnv{goos: "linux", lookPath: lookPathWith("dnf")}, "sudo", []string{"dnf", "install", "-y", "tmux"}, true},
		{"linux brew", tmuxEnv{goos: "linux", lookPath: lookPathWith("brew")}, "brew", []string{"install", "tmux"}, true},
		{"darwin brew", tmuxEnv{goos: "darwin", lookPath: lookPathWith("brew")}, "brew", []string{"install", "tmux"}, true},
		{"darwin none", tmuxEnv{goos: "darwin", lookPath: lookPathWith()}, "", nil, false},
		{"windows", tmuxEnv{goos: "windows", lookPath: lookPathWith("brew", "apt-get")}, "", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			name, args, ok := tmuxInstallCommand(c.env)
			if ok != c.wantOK || name != c.wantName || !reflect.DeepEqual(args, c.wantArgs) {
				t.Fatalf("got (%q, %v, %v), want (%q, %v, %v)", name, args, ok, c.wantName, c.wantArgs, c.wantOK)
			}
		})
	}
}

func TestTmuxFix_InstallsAndRechecks(t *testing.T) {
	installed := false
	var gotCmd []string
	env := tmuxEnv{
		goos: "linux",
		lookPath: func(name string) (string, error) {
			if name == "tmux" && !installed {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + name, nil
		},
		runner:     fakeTmuxRunner{out: "tmux 3.4\n"},
		isTerminal: func() bool { return true },
		install: func(name string, args ...string) error {
			gotCmd = append([]string{name}, args...)
			installed = true
			return nil
		},
	}
	fixed, detail := tmuxCheck(env).Fix()
	if !fixed {
		t.Fatalf("expected fixed, detail=%q", detail)
	}
	want := []string{"sudo", "apt-get", "install", "-y", "tmux"}
	if !reflect.DeepEqual(gotCmd, want) {
		t.Fatalf("install got %v, want %v", gotCmd, want)
	}
}

func TestTmuxFix_NoPackageManager(t *testing.T) {
	called := false
	env := tmuxEnv{
		goos:     "linux",
		lookPath: lookPathWith(),
		runner:   fakeTmuxRunner{},
		install:  func(string, ...string) error { called = true; return nil },
	}
	fixed, detail := tmuxCheck(env).Fix()
	if fixed || !strings.Contains(detail, "install tmux ≥ 3.1 manually") {
		t.Fatalf("got (%v, %q)", fixed, detail)
	}
	if called {
		t.Fatal("install must not be called")
	}
}

func TestTmuxFix_InstallError(t *testing.T) {
	env := tmuxEnv{
		goos:       "linux",
		lookPath:   lookPathWith("apt-get"),
		runner:     fakeTmuxRunner{},
		isTerminal: func() bool { return true },
		install:    func(string, ...string) error { return errors.New("exit status 100") },
	}
	fixed, detail := tmuxCheck(env).Fix()
	if fixed || detail != "install failed: exit status 100" {
		t.Fatalf("got (%v, %q)", fixed, detail)
	}
}

func TestTmuxFix_AlreadyInstalled(t *testing.T) {
	env := tmuxEnv{goos: "linux", lookPath: lookPathWith("tmux"), runner: fakeTmuxRunner{out: "tmux 3.4"}}
	fixed, detail := tmuxCheck(env).Fix()
	if !fixed || detail != "already installed" {
		t.Fatalf("got (%v, %q)", fixed, detail)
	}
}

func TestPrintTmuxHint(t *testing.T) {
	var buf bytes.Buffer
	printTmuxHint(&buf, tmuxEnv{goos: "linux", lookPath: lookPathWith(), runner: fakeTmuxRunner{}})
	if !strings.Contains(buf.String(), "tmux not found") {
		t.Fatalf("hint missing: %q", buf.String())
	}
	buf.Reset()
	printTmuxHint(&buf, tmuxEnv{goos: "linux", lookPath: lookPathWith("tmux"), runner: fakeTmuxRunner{out: "tmux 3.4"}})
	if buf.Len() != 0 {
		t.Fatalf("expected silence when ok, got %q", buf.String())
	}
}

func TestTmuxFix_SudoWithoutTerminal(t *testing.T) {
	called := false
	env := tmuxEnv{
		goos:       "linux",
		lookPath:   lookPathWith("apt-get"),
		runner:     fakeTmuxRunner{},
		isTerminal: func() bool { return false },
		install:    func(string, ...string) error { called = true; return nil },
	}
	fixed, detail := tmuxCheck(env).Fix()
	if fixed || called {
		t.Fatalf("fixed=%v called=%v, want neither", fixed, called)
	}
	for _, want := range []string{"sudo apt-get install -y tmux", "in your own terminal"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q missing %q", detail, want)
		}
	}
}

func TestTmuxFix_BrewWithoutTerminalStillInstalls(t *testing.T) {
	called := false
	env := tmuxEnv{
		goos:       "darwin",
		lookPath:   lookPathWith("brew"),
		runner:     fakeTmuxRunner{},
		isTerminal: func() bool { return false },
		install:    func(string, ...string) error { called = true; return errors.New("boom") },
	}
	if _, detail := tmuxCheck(env).Fix(); !called || detail != "install failed: boom" {
		t.Fatalf("called=%v detail=%q", called, detail)
	}
}

// The installer must never write to stdout: `zenify doctor --fix --json > out.json`
// would otherwise mix package-manager output into the JSON.
func TestInstallCmd_StdoutGoesToStderr(t *testing.T) {
	cmd := installCmd("brew", "install", "tmux")
	if cmd.Stdout != os.Stderr {
		t.Fatalf("Stdout = %v, want os.Stderr", cmd.Stdout)
	}
	if cmd.Stdin != os.Stdin || cmd.Stderr != os.Stderr {
		t.Fatalf("Stdin/Stderr must stay attached to the terminal")
	}
}
