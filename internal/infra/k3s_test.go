package infra

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
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

// isFileProbe matches readRootFile's existence probe; it returns the path.
func isProbe(c call) bool { _, ok := isFileProbe(c); return ok }

func isFileProbe(c call) (string, bool) {
	if c.name == "sudo" && len(c.args) == 6 && c.args[0] == "-n" && c.args[1] == "sh" && c.args[2] == "-c" {
		return c.args[5], true
	}
	return "", false
}

// hostFiles fakes readRootFile's probe / `sudo cat` over an in-memory file map.
func hostFiles(files map[string]string, k3sVersion string) func(call) ([]byte, error) {
	return func(c call) ([]byte, error) {
		switch {
		case c.name == "k3s" && len(c.args) == 1 && c.args[0] == "--version":
			if k3sVersion == "" {
				return nil, exec.ErrNotFound
			}
			return []byte("k3s version " + k3sVersion + " (abc)\ngo version go1.26\n"), nil
		case isProbe(c):
			p, _ := isFileProbe(c)
			if _, ok := files[p]; ok {
				return nil, nil
			}
			return nil, exitErr(rootFileAbsent)
		case c.name == "sudo" && c.args[0] == "cat":
			return []byte(files[c.args[1]]), nil
		case c.name == "kubectl" && len(c.args) > 2 && c.args[2] == "get" && c.args[3] == "nodes":
			return []byte("True\n"), nil
		case c.name == "kubectl" && len(c.args) > 3 && c.args[2] == "get" && c.args[3] == "crd":
			return []byte("True"), nil
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
	files[mirrorHostsPath] = "[host.\"" + mirrorEndpoint() + "\"]\n  capabilities = [\"pull\", \"resolve\"]\n"
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
	if r.ran("sudo env INSTALL_K3S_VERSION") || r.ran("sudo tee") || r.ran("sudo systemctl restart") {
		t.Fatalf("unexpected mutation: %v", r.calls)
	}
}

// Business rule: a host config change (here the registry mirror) must reach k3s,
// which reads registries.yaml only at start; the installer at the same version
// does not restart the service, so InstallK3s restarts it.
func TestInstallK3sChangedFileRestartsK3s(t *testing.T) {
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
	if !r.ran("sudo tee " + registriesPath + ".zenify-") {
		t.Fatalf("registries.yaml not written: %v", r.calls)
	}
	if !r.ran("sudo systemctl restart k3s") || !r.ran("kubectl --kubeconfig "+K3sKubeconfig+" get nodes") {
		t.Fatalf("k3s not restarted and awaited: %v", r.calls)
	}
	if !r.ran("sudo rm -f " + restartMarker) {
		t.Fatalf("marker not cleared: %v", r.calls)
	}
}

// Business rule: a version mismatch runs the pinned installer.
func TestInstallK3sOtherVersionRunsPinnedInstaller(t *testing.T) {
	r := &fakeRunner{respond: hostFiles(desiredHostFiles(t), "v1.30.0+k3s1")}
	if _, err := InstallK3s(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if !r.ran("sudo env INSTALL_K3S_VERSION=" + K3sVersion + " sh -s -") {
		t.Fatalf("pinned installer not run: %v", r.calls)
	}
}

// Business rule: files matching is not enough — if containerd never loaded the
// mirror (k3s not restarted since), image pulls from the alias fail, so restart.
func TestInstallK3sMirrorNotLoadedRestarts(t *testing.T) {
	files := desiredHostFiles(t)
	delete(files, mirrorHostsPath)
	loaded := false
	base := hostFiles(files, K3sVersion)
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.name == "sudo" && c.args[0] == "systemctl" && c.args[1] == "restart" && c.args[2] == "k3s" {
			loaded = true
			files[mirrorHostsPath] = desiredHostFiles(t)[mirrorHostsPath]
		}
		return base(c)
	}}
	changed, err := InstallK3s(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !loaded || r.ran("sudo tee") {
		t.Fatalf("changed=%v restarted=%v calls=%v", changed, loaded, r.calls)
	}
}

// Business rule: a failed restart keeps the marker so the next run retries it.
func TestInstallK3sFailedRestartKeepsMarker(t *testing.T) {
	files := desiredHostFiles(t)
	delete(files, registriesPath)
	base := hostFiles(files, K3sVersion)
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.name == "sudo" && c.args[0] == "systemctl" && c.args[1] == "restart" && c.args[2] == "k3s" {
			return nil, errors.New("job failed")
		}
		return base(c)
	}}
	if _, err := InstallK3s(context.Background(), r); err == nil || !strings.Contains(err.Error(), "restart k3s") {
		t.Fatalf("err = %v", err)
	}
	if !r.ran("sudo tee "+restartMarker) || r.ran("sudo rm -f "+restartMarker) {
		t.Fatalf("marker not kept: %v", r.calls)
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
		if c.name == "sudo" && c.args[0] == "tee" && strings.HasPrefix(c.args[1], k3sConfigPath+".zenify-") {
			return nil, errors.New("disk full")
		}
		return base(c)
	}}
	_, err := InstallK3s(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), k3sConfigPath) || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v", err)
	}
}

// Business rule: if the installer failed after a config change, the next run
// must still restart k3s even though the files now match.
func TestInstallK3sRetriesPendingRestart(t *testing.T) {
	files := desiredHostFiles(t)
	files[restartMarker] = "k3s restart pending\n"
	r := &fakeRunner{respond: hostFiles(files, K3sVersion)}
	changed, err := InstallK3s(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !r.ran("sudo systemctl restart k3s") || !r.ran("sudo rm -f "+restartMarker) {
		t.Fatalf("changed=%v calls=%v", changed, r.calls)
	}
}

// Business rule: a failed installer keeps the marker so the restart is retried.
func TestInstallK3sFailedInstallerKeepsMarker(t *testing.T) {
	files := desiredHostFiles(t)
	delete(files, registriesPath)
	base := hostFiles(files, "v1.30.0+k3s1")
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.name == "sudo" && c.args[0] == "env" {
			return nil, errors.New("installer failed")
		}
		return base(c)
	}}
	if _, err := InstallK3s(context.Background(), r); err == nil {
		t.Fatal("want installer error")
	}
	if !r.ran("sudo tee "+restartMarker) || r.ran("sudo rm -f "+restartMarker) {
		t.Fatalf("marker not kept: %v", r.calls)
	}
}

// Business rule: a firewall unit left inactive by a failed earlier run is restarted.
func TestInstallK3sInactiveFirewallRestarts(t *testing.T) {
	base := hostFiles(desiredHostFiles(t), K3sVersion)
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.name == "sudo" && c.args[0] == "systemctl" && c.args[1] == "is-active" {
			return nil, exitErr(3)
		}
		return base(c)
	}}
	if _, err := InstallK3s(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if !r.ran("sudo systemctl restart " + firewallUnit) {
		t.Fatalf("firewall not restarted: %v", r.calls)
	}
}

// index returns the position of the first call with the given line prefix at
// or after from, or -1.
func (f *fakeRunner) index(prefix string, from int) int {
	for i := from; i < len(f.calls); i++ {
		if strings.HasPrefix(f.calls[i].line(), prefix) {
			return i
		}
	}
	return -1
}

// Business rule: on a fresh host the installer's own start must read the final
// config and run behind the firewall, and nothing may restart k3s while it is
// still bootstrapping: a restart then left addons.k3s.cattle.io stuck at
// Established=False and k3s crash-looping (fresh VM, 2026-11-26).
func TestInstallK3sFreshInstallNoRestartWhileBootstrapping(t *testing.T) {
	files := map[string]string{}
	installed := ""
	base := writable(files)
	r := &fakeRunner{}
	r.respond = func(c call) ([]byte, error) {
		switch {
		case c.name == "k3s" && len(c.args) == 1 && c.args[0] == "--version":
			if installed == "" {
				return nil, exec.ErrNotFound
			}
			return []byte("k3s version " + installed + " (abc)\n"), nil
		case c.name == "sudo" && len(c.args) > 1 && c.args[0] == "env":
			installed = K3sVersion
			files[mirrorHostsPath] = desiredHostFiles(t)[mirrorHostsPath]
		}
		return base(c)
	}
	if _, err := InstallK3s(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	installer := r.index("sudo env INSTALL_K3S_VERSION="+K3sVersion, 0)
	if installer < 0 {
		t.Fatalf("installer not run: %v", r.calls)
	}
	for _, f := range k3sHostFiles() {
		if w := r.index("sudo tee "+f.path+".zenify-", 0); w < 0 || w > installer {
			t.Fatalf("%s not written before the installer: %v", f.path, r.calls)
		}
	}
	if fw := r.index("sudo systemctl restart "+firewallUnit, 0); fw < 0 || fw > installer {
		t.Fatalf("firewall not active before the installer: %v", r.calls)
	}
	if r.index("sudo systemctl restart", installer) >= 0 {
		t.Fatalf("k3s restarted after its first start: %v", r.calls)
	}
	nodes := r.index("kubectl --kubeconfig "+K3sKubeconfig+" get nodes", installer)
	for _, crd := range k3sCRDs {
		if r.index("kubectl --kubeconfig "+K3sKubeconfig+" get crd "+crd, nodes) < 0 || nodes < 0 {
			t.Fatalf("installer start not awaited (node Ready, CRD %s): %v", crd, r.calls)
		}
	}
}

// Business rule: a restart for a config change waits for the running k3s to
// finish bootstrapping first (CRDs Established), then waits again after.
func TestInstallK3sRestartWaitsForBootstrap(t *testing.T) {
	files := desiredHostFiles(t)
	delete(files, registriesPath)
	r := &fakeRunner{respond: hostFiles(files, K3sVersion)}
	if _, err := InstallK3s(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	restart := r.index("sudo systemctl restart k3s", 0)
	before := r.index("kubectl --kubeconfig "+K3sKubeconfig+" get crd", 0)
	after := r.index("kubectl --kubeconfig "+K3sKubeconfig+" get crd", restart)
	if restart < 0 || before < 0 || before > restart || after < 0 {
		t.Fatalf("restart=%d crd-before=%d crd-after=%d: %v", restart, before, after, r.calls)
	}
}

// Business rule: a k3s CRD stuck at Established=False is named with its manual
// recovery, and the kit never deletes a CRD itself.
func TestInstallK3sStuckCRDNamedError(t *testing.T) {
	oldPoll, oldCRD := pollInterval, crdEstablishedTimeout
	pollInterval, crdEstablishedTimeout = time.Millisecond, 20*time.Millisecond
	defer func() { pollInterval, crdEstablishedTimeout = oldPoll, oldCRD }()
	files := desiredHostFiles(t)
	files[restartMarker] = "k3s restart pending\n"
	base := hostFiles(files, K3sVersion)
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.name == "kubectl" && strings.Contains(c.line(), "get crd addons.k3s.cattle.io") {
			return []byte("False"), nil
		}
		return base(c)
	}}
	_, err := InstallK3s(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "addons.k3s.cattle.io") ||
		!strings.Contains(err.Error(), "delete crd addons.k3s.cattle.io") || !strings.Contains(err.Error(), "sudo systemctl restart k3s") {
		t.Fatalf("err = %v", err)
	}
	if r.ran("sudo systemctl restart k3s") || r.ran("kubectl --kubeconfig "+K3sKubeconfig+" delete") {
		t.Fatalf("restarted a bootstrapping k3s or deleted a CRD: %v", r.calls)
	}
}
