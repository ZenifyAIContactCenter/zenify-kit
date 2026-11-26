package infra

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Requirements are the minimum host resources for the local k3s devbox.
type Requirements struct {
	// MinMemBytes is the RAM floor per profile: buildkitd is OOM-killed by a
	// web build next to the full env on less.
	MinMemBytes  map[string]uint64
	MinDiskBytes uint64
	// MinDiskResumeBytes applies once k3s is installed: its images already use
	// part of MinDiskBytes, so a re-run only needs room for new builds.
	MinDiskResumeBytes uint64
}

// DefaultRequirements: 12 GiB RAM for core and 16 GiB for full, 30 GiB free where /var/lib/rancher lives
// for a fresh install, 10 GiB to re-run on an installed devbox.
var DefaultRequirements = Requirements{MinMemBytes: map[string]uint64{ProfileCore: 12884901888, ProfileFull: 17179869184}, MinDiskBytes: 32212254720, MinDiskResumeBytes: 10737418240}

// Problem is one failed preflight check with how to fix it.
type Problem struct {
	Check, Detail, Fix string
}

const (
	rancherDir    = "/var/lib/rancher"
	k3sConfigPath = "/etc/rancher/k3s/config.yaml"
	npmKeyName    = "NPM_TOKEN"
)

// HostProbe abstracts every host fact Preflight reads, so tests can fake it.
type HostProbe interface {
	GOOS() string
	HasSystemd() bool
	CanSudo() bool
	MemTotalBytes() (uint64, error)
	DiskFreeBytes(path string) (uint64, error)
	// InstalledK3sVersion returns the installed k3s version and whether k3s is
	// installed. An error means the probe itself failed (k3s present but unusable).
	InstalledK3sVersion() (version string, installed bool, err error)
	// K3sServicelbDisabled reports whether the k3s config disables servicelb.
	// A missing config means not disabled; an unreadable or invalid one is an error.
	K3sServicelbDisabled() (bool, error)
	EnvSet(key string) bool
	HasCommand(name string) bool
	// GhLoggedIn returns nil when `gh auth status` succeeds, else the failure with gh's stderr.
	GhLoggedIn() error
	// GhTokenAvailable returns nil when `gh auth token` yields a token (never exposed).
	GhTokenAvailable() error
}

// tools `infra up` needs, in check order, with the install hint for each.
var requiredTools = []struct{ cmd, fix string }{
	{"curl", "sudo apt install -y curl"},
	{"git", "sudo apt install -y git"},
	{"gh", "install GitHub CLI (https://cli.github.com), then run: gh auth login"},
	{"certutil", "sudo apt install -y libnss3-tools"},
	{"nft", "sudo apt install -y nftables"},
	{"resolvectl", "enable systemd-resolved: sudo apt install -y systemd-resolved"},
}

// Preflight returns every failed check in a fixed order; empty means the host is ready.
// wantK3s is the pinned k3s version (Task 10 passes its K3sVersion constant).
// profile (core or full) picks the RAM floor.
func Preflight(h HostProbe, r Requirements, wantK3s, profile string) []Problem {
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
	} else if need := r.MinMemBytes[profile]; mem < need {
		add("memory", fmt.Sprintf("profile %s needs %d bytes RAM (%d GiB; core needs %d, full needs %d), have %d",
			profile, need, need>>30, r.MinMemBytes[ProfileCore], r.MinMemBytes[ProfileFull], mem),
			"use a host with more memory, or run up --profile core")
	}

	ver, installed, k3sErr := h.InstalledK3sVersion()
	minDisk := r.MinDiskBytes
	if k3sErr == nil && installed {
		minDisk = r.MinDiskResumeBytes
	}
	if free, err := h.DiskFreeBytes(rancherDir); err != nil {
		add("disk", "cannot read free disk for "+rancherDir+": "+err.Error(), "check the filesystem is mounted")
	} else if free < minDisk {
		add("disk", fmt.Sprintf("need %d bytes free for %s, have %d", minDisk, rancherDir, free), "free disk space or grow the volume")
	}

	if err := k3sErr; err != nil {
		add("k3s", "cannot probe installed k3s: "+err.Error(), "check the k3s binary runs: k3s --version")
	} else if installed {
		if ver != wantK3s {
			add("k3s", fmt.Sprintf("k3s %s installed, need %s", ver, wantK3s), "uninstall it (/usr/local/bin/k3s-uninstall.sh) so infra up installs the pinned version")
		} else if disabled, err := h.K3sServicelbDisabled(); err != nil {
			add("k3s", "cannot read "+k3sConfigPath+": "+err.Error(), "fix the file so it is readable valid YAML")
		} else if !disabled {
			add("k3s", "k3s servicelb is not disabled", "add 'disable: [servicelb]' to "+k3sConfigPath+" (k3s flag --disable=servicelb) and restart k3s")
		}
	}

	if !h.EnvSet(npmKeyName) {
		if err := h.GhTokenAvailable(); err != nil {
			add("build-secrets", npmKeyName+" is not set and gh auth token failed: "+err.Error(), "export "+npmKeyName+" or run: gh auth login")
		}
	}

	for _, t := range requiredTools {
		if !h.HasCommand(t.cmd) {
			add("tools", t.cmd+" not found", t.fix)
		}
	}
	if h.HasCommand("gh") {
		if err := h.GhLoggedIn(); err != nil {
			add("tools", "gh is not logged in: "+err.Error(), "run: gh auth login")
		}
	}
	return ps
}

type realProbe struct{}

// RealHostProbe probes the actual host.
func RealHostProbe() HostProbe { return realProbe{} }

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
		parent := filepath.Dir(p)
		if parent == p {
			return 0, fmt.Errorf("infra: no existing parent for %s", path)
		}
		p = parent
	}
}

func (realProbe) InstalledK3sVersion() (string, bool, error) {
	out, err := exec.Command("k3s", "--version").Output()
	if errors.Is(err, exec.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", true, fmt.Errorf("k3s --version: %w", withStderr(err))
	}
	v, err := parseK3sVersion(out)
	return v, true, err
}

// parseK3sVersion reads `k3s --version`; first line: "k3s version v1.30.0+k3s1 (hash)".
func parseK3sVersion(out []byte) (string, error) {
	f := strings.Fields(string(out))
	if len(f) >= 3 && f[0] == "k3s" {
		return f[2], nil
	}
	return "", fmt.Errorf("unexpected k3s --version output %q", strings.TrimSpace(string(out)))
}

func (realProbe) K3sServicelbDisabled() (bool, error) {
	b, err := os.ReadFile(k3sConfigPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", k3sConfigPath, err)
	}
	return configDisablesServicelb(b)
}

// configDisablesServicelb parses a k3s config.yaml; disable may be a list or a
// comma-separated string.
func configDisablesServicelb(cfg []byte) (bool, error) {
	var doc struct {
		Disable any `yaml:"disable"`
	}
	if err := yaml.Unmarshal(cfg, &doc); err != nil {
		return false, fmt.Errorf("parse yaml: %w", err)
	}
	var items []string
	switch v := doc.Disable.(type) {
	case nil:
	case string:
		items = strings.Split(v, ",")
	case []any:
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				return false, fmt.Errorf("disable: list item %v is not a string", e)
			}
			items = append(items, strings.Split(s, ",")...)
		}
	default:
		return false, fmt.Errorf("disable: unsupported type %T", v)
	}
	for _, it := range items {
		if strings.TrimSpace(it) == "servicelb" {
			return true, nil
		}
	}
	return false, nil
}

func (realProbe) EnvSet(key string) bool { return os.Getenv(key) != "" }

func (realProbe) HasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (realProbe) GhLoggedIn() error {
	_, err := exec.Command("gh", "auth", "status").Output()
	return withStderr(err)
}

func (realProbe) GhTokenAvailable() error {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return withStderr(err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return errors.New("gh auth token returned an empty token")
	}
	return nil
}

// withStderr appends a command's trimmed stderr to its exec error (nil stays nil).
func withStderr(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
	}
	return err
}
