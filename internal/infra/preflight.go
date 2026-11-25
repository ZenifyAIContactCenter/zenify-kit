package infra

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
)

// Requirements are the minimum host resources for the local k3s devbox.
type Requirements struct {
	MinMemBytes  uint64
	MinDiskBytes uint64
}

// DefaultRequirements: 6 GiB RAM, 30 GiB free where /var/lib/rancher lives.
var DefaultRequirements = Requirements{MinMemBytes: 6442450944, MinDiskBytes: 32212254720}

// Problem is one failed preflight check with how to fix it.
type Problem struct {
	Check, Detail, Fix string
}

const (
	rancherDir       = "/var/lib/rancher"
	k3sConfigPath    = "/etc/rancher/k3s/config.yaml"
	npmKeyName       = "NPM_TOKEN"
	taskforceKeyName = "NPM_TASKFORCESH_TOKEN" //nolint:gosec // env key name, not a credential
)

// HostProbe abstracts every host fact Preflight reads, so tests can fake it.
type HostProbe interface {
	GOOS() string
	HasSystemd() bool
	CanSudo() bool
	MemTotalBytes() (uint64, error)
	DiskFreeBytes(path string) (uint64, error)
	// InstalledK3sVersion returns the installed k3s version and whether k3s is installed.
	InstalledK3sVersion() (string, bool)
	// K3sServicelbDisabled returns whether servicelb is disabled and whether k3s is installed.
	K3sServicelbDisabled() (disabled, installed bool)
	EnvSet(key string) bool
	HasCommand(name string) bool
	GhLoggedIn() bool
	// GhTokenAvailable reports whether `gh auth token` yields a token.
	GhTokenAvailable() bool
	// LookupKey reports whether key is set in the process env or the kit's personal env store.
	LookupKey(name string) bool
}

// tools `infra up` needs, in check order, with the install hint for each.
var requiredTools = []struct{ cmd, fix string }{
	{"curl", "sudo apt install -y curl"},
	{"git", "sudo apt install -y git"},
	{"gh", "install GitHub CLI (https://cli.github.com), then run: gh auth login"},
	{"certutil", "sudo apt install -y libnss3-tools"},
}

// Preflight returns every failed check in a fixed order; empty means the host is ready.
// wantK3s is the pinned k3s version (Task 10 passes its K3sVersion constant).
func Preflight(h HostProbe, r Requirements, wantK3s string) []Problem {
	var ps []Problem
	add := func(check, detail, fix string) { ps = append(ps, Problem{check, detail, fix}) }

	if host := h.GOOS(); host != "linux" {
		add("os", fmt.Sprintf("host OS is %s, infra up supports linux only", host), "run on an Ubuntu Server devbox")
	}
	if !h.HasSystemd() {
		add("systemd", "systemd is not the init system", "use a distro with systemd (Ubuntu Server)")
	}
	if !h.CanSudo() {
		add("sudo", "sudo needs a password or is unavailable", "grant passwordless sudo: echo \"$USER ALL=(ALL) NOPASSWD:ALL\" | sudo tee /etc/sudoers.d/$USER")
	}

	if mem, err := h.MemTotalBytes(); err != nil {
		add("memory", "cannot read total memory: "+err.Error(), "check /proc/meminfo is readable")
	} else if mem < r.MinMemBytes {
		add("memory", fmt.Sprintf("need %d bytes RAM, have %d", r.MinMemBytes, mem), "use a host with more memory")
	}

	if free, err := h.DiskFreeBytes(rancherDir); err != nil {
		add("disk", "cannot read free disk for "+rancherDir+": "+err.Error(), "check the filesystem is mounted")
	} else if free < r.MinDiskBytes {
		add("disk", fmt.Sprintf("need %d bytes free for %s, have %d", r.MinDiskBytes, rancherDir, free), "free disk space or grow the volume")
	}

	if ver, installed := h.InstalledK3sVersion(); installed {
		if ver != wantK3s {
			add("k3s", fmt.Sprintf("k3s %s installed, need %s", ver, wantK3s), "uninstall it (/usr/local/bin/k3s-uninstall.sh) so infra up installs the pinned version")
		} else if disabled, _ := h.K3sServicelbDisabled(); !disabled {
			add("k3s", "k3s servicelb is not disabled", "add 'disable: [servicelb]' to "+k3sConfigPath+" (k3s flag --disable=servicelb) and restart k3s")
		}
	}

	if !h.EnvSet(npmKeyName) && !h.GhTokenAvailable() {
		add("build-secrets", npmKeyName+" is not set and gh auth token returned none", "export "+npmKeyName+" or run: gh auth login")
	}
	if !h.LookupKey(taskforceKeyName) {
		add("build-secrets", taskforceKeyName+" is not set in the env or the zenify env store", "run: zenify env set "+taskforceKeyName)
	}

	for _, t := range requiredTools {
		if !h.HasCommand(t.cmd) {
			add("tools", t.cmd+" not found", t.fix)
		}
	}
	if h.HasCommand("gh") && !h.GhLoggedIn() {
		add("tools", "gh is not logged in", "run: gh auth login")
	}
	return ps
}

type realProbe struct{ workspace string }

// RealHostProbe probes the actual host. workspace locates the kit's personal
// env store (<workspace>/.zenify/.env) for LookupKey; the caller resolves it.
func RealHostProbe(workspace string) HostProbe { return realProbe{workspace: workspace} }

func (realProbe) GOOS() string { return goos }

func (realProbe) HasSystemd() bool {
	fi, err := os.Stat("/run/systemd/system")
	return err == nil && fi.IsDir()
}

func (realProbe) CanSudo() bool { return exec.Command("sudo", "-n", "true").Run() == nil }

func (realProbe) MemTotalBytes() (uint64, error) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("infra: read /proc/meminfo: %w", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "MemTotal:" {
			kb, err := strconv.ParseUint(f[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("infra: parse MemTotal %q: %w", f[1], err)
			}
			return kb * 1024, nil
		}
	}
	return 0, fmt.Errorf("infra: MemTotal not found in /proc/meminfo")
}

// DiskFreeBytes measures path, or its nearest existing parent when path does not exist yet.
func (realProbe) DiskFreeBytes(path string) (uint64, error) {
	for p := path; ; {
		if _, err := os.Stat(p); err == nil {
			return statfsFree(p)
		}
		parent := parentDir(p)
		if parent == p {
			return 0, fmt.Errorf("infra: no existing parent for %s", path)
		}
		p = parent
	}
}

func (realProbe) InstalledK3sVersion() (string, bool) {
	out, err := exec.Command("k3s", "--version").Output()
	if err != nil {
		return "", false
	}
	// first line: "k3s version v1.30.0+k3s1 (hash)"
	f := strings.Fields(string(out))
	if len(f) >= 3 && f[0] == "k3s" {
		return f[2], true
	}
	return strings.TrimSpace(string(out)), true
}

func (realProbe) K3sServicelbDisabled() (disabled, installed bool) {
	if _, err := exec.LookPath("k3s"); err != nil {
		return false, false
	}
	b, err := os.ReadFile(k3sConfigPath)
	if err != nil {
		return false, true
	}
	return configDisablesServicelb(string(b)), true
}

// configDisablesServicelb scans a k3s config.yaml for servicelb in its disable: list
// (block list, inline list or scalar form).
func configDisablesServicelb(cfg string) bool {
	inDisable := false
	for _, line := range strings.Split(cfg, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "disable:") {
			rest := strings.TrimSpace(strings.TrimPrefix(t, "disable:"))
			if rest == "" {
				inDisable = true
				continue
			}
			return strings.Contains(rest, "servicelb")
		}
		if inDisable {
			if strings.HasPrefix(t, "-") {
				if strings.Contains(t, "servicelb") {
					return true
				}
				continue
			}
			if t != "" && !strings.HasPrefix(t, "#") {
				inDisable = false
			}
		}
	}
	return false
}

func (realProbe) EnvSet(key string) bool { return os.Getenv(key) != "" }

func (realProbe) HasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (realProbe) GhLoggedIn() bool { return exec.Command("gh", "auth", "status").Run() == nil }

func (realProbe) GhTokenAvailable() bool {
	out, err := exec.Command("gh", "auth", "token").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func (p realProbe) LookupKey(name string) bool {
	f, err := devenv.Load(devenv.Path(p.workspace))
	if err != nil {
		// unreadable store: fall back to the process env only; Preflight then reports the key missing
		f = nil
	}
	return devenv.Lookup(os.Getenv, f)(name) != ""
}
