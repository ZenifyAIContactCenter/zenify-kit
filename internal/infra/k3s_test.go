package infra

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
)

type call struct {
	name  string
	args  []string
	stdin string
}

func (c call) line() string { return strings.Join(append([]string{c.name}, c.args...), " ") }

// fakeRunner records every call; respond decides the output (nil → empty, no error).
type fakeRunner struct {
	calls   []call
	respond func(c call) ([]byte, error)
}

func (f *fakeRunner) Run(_ context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	c := call{name: name, args: args}
	if stdin != nil {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, err
		}
		c.stdin = string(b)
	}
	f.calls = append(f.calls, c)
	if f.respond == nil {
		return nil, nil
	}
	return f.respond(c)
}

func (f *fakeRunner) ran(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c.line(), prefix) {
			return true
		}
	}
	return false
}

type exitErr int

func (e exitErr) Error() string { return "exit status" }
func (e exitErr) ExitCode() int { return int(e) }

// hostFiles fakes `sudo test -f` / `sudo cat` over an in-memory file map.
func hostFiles(files map[string]string, k3sVersion string) func(call) ([]byte, error) {
	return func(c call) ([]byte, error) {
		switch {
		case c.name == "k3s" && len(c.args) == 1 && c.args[0] == "--version":
			if k3sVersion == "" {
				return nil, exec.ErrNotFound
			}
			return []byte("k3s version " + k3sVersion + " (abc)\ngo version go1.26\n"), nil
		case c.name == "sudo" && c.args[0] == "test":
			if _, ok := files[c.args[2]]; ok {
				return nil, nil
			}
			return nil, exitErr(1)
		case c.name == "sudo" && c.args[0] == "cat":
			return []byte(files[c.args[1]]), nil
		}
		return nil, nil
	}
}

func desiredHostFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, f := range k3sHostFiles() {
		b, err := f.render()
		if err != nil {
			t.Fatal(err)
		}
		files[f.path] = string(b)
	}
	return files
}

// Business rule: re-running infra up on a ready host must not reinstall or restart k3s.
func TestInstallK3sUpToDateIsNoop(t *testing.T) {
	r := &fakeRunner{respond: hostFiles(desiredHostFiles(t), K3sVersion)}
	changed, err := InstallK3s(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("changed = true on an up-to-date host")
	}
	if r.ran("sudo env INSTALL_K3S_VERSION") || r.ran("sudo tee") || r.ran("sudo systemctl") {
		t.Fatalf("unexpected mutation: %v", r.calls)
	}
}

// Business rule: a host config change (here the registry mirror) must reach k3s,
// which reads registries.yaml only at start, so the pinned installer reruns.
func TestInstallK3sChangedFileRunsPinnedInstaller(t *testing.T) {
	files := desiredHostFiles(t)
	delete(files, registriesPath)
	r := &fakeRunner{respond: hostFiles(files, K3sVersion)}
	changed, err := InstallK3s(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("changed = false after writing registries.yaml")
	}
	if !r.ran("sudo tee " + registriesPath + ".zenify.tmp") {
		t.Fatalf("registries.yaml not written: %v", r.calls)
	}
	if !r.ran("sudo env INSTALL_K3S_VERSION=" + K3sVersion + " sh -s -") {
		t.Fatalf("pinned installer not run: %v", r.calls)
	}
}

// Business rule: a missing firewall leaves the API server open to the LAN, so its unit is (re)started.
func TestInstallK3sFirewallChangeRestartsUnit(t *testing.T) {
	files := desiredHostFiles(t)
	delete(files, nftPath)
	r := &fakeRunner{respond: hostFiles(files, K3sVersion)}
	if _, err := InstallK3s(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if !r.ran("sudo systemctl restart " + firewallUnit) {
		t.Fatalf("firewall unit not restarted: %v", r.calls)
	}
	if r.ran("sudo env INSTALL_K3S_VERSION") {
		t.Fatal("firewall-only change must not rerun the k3s installer")
	}
}

func TestInstallK3sStepErrorNamesStep(t *testing.T) {
	files := desiredHostFiles(t)
	delete(files, k3sConfigPath)
	base := hostFiles(files, K3sVersion)
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.name == "sudo" && c.args[0] == "tee" {
			return nil, errors.New("disk full")
		}
		return base(c)
	}}
	_, err := InstallK3s(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), k3sConfigPath) || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v", err)
	}
}
