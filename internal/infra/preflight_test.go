package infra

import (
	"errors"
	"strings"
	"testing"
)

const testPin = "v1.31.0+k3s1"

type fakeProbe struct {
	os         string
	systemd    bool
	sudo       bool
	mem, disk  uint64
	memErr     error
	diskE      error
	k3sVer     string
	k3sInstall bool
	k3sErr     error
	servicelb  bool
	cfgErr     error
	env        map[string]string
	cmds       map[string]bool
	ghStatus   error
	ghToken    error
}

func (f fakeProbe) GOOS() string                         { return f.os }
func (f fakeProbe) HasSystemd() bool                     { return f.systemd }
func (f fakeProbe) CanSudo() bool                        { return f.sudo }
func (f fakeProbe) MemTotalBytes() (uint64, error)       { return f.mem, f.memErr }
func (f fakeProbe) DiskFreeBytes(string) (uint64, error) { return f.disk, f.diskE }
func (f fakeProbe) InstalledK3sVersion() (string, bool, error) {
	return f.k3sVer, f.k3sInstall, f.k3sErr
}
func (f fakeProbe) K3sServicelbDisabled() (bool, error) { return f.servicelb, f.cfgErr }
func (f fakeProbe) EnvSet(k string) bool                { return f.env[k] != "" }
func (f fakeProbe) HasCommand(n string) bool            { return f.cmds[n] }
func (f fakeProbe) GhLoggedIn() error                   { return f.ghStatus }
func (f fakeProbe) GhTokenAvailable() error             { return f.ghToken }

func okProbe() fakeProbe {
	return fakeProbe{
		os: "linux", systemd: true, sudo: true,
		mem: DefaultRequirements.MinMemBytes[ProfileFull], disk: DefaultRequirements.MinDiskBytes,
		env:  map[string]string{"NPM_TOKEN": "x"},
		cmds: map[string]bool{"curl": true, "git": true, "gh": true, "certutil": true, "nft": true, "resolvectl": true},
	}
}

func TestPreflight(t *testing.T) {
	tests := []struct {
		name      string
		mod       func(*fakeProbe)
		wantCheck string // empty = no problems
		wantIn    []string
		wantNotIn []string
	}{
		{name: "all ok", mod: func(*fakeProbe) {}},
		{name: "darwin", mod: func(p *fakeProbe) { p.os = "darwin" }, wantCheck: "os"},
		{name: "low memory", mod: func(p *fakeProbe) { p.mem = 1000 }, wantCheck: "memory", wantIn: []string{"12884901888", "1000", "profile core"}},
		// 8 GiB VM failed the web build with core only; 12 GiB is enough for core but not for full
		{name: "8 GiB fails core", mod: func(p *fakeProbe) { p.mem = 8 << 30 }, wantCheck: "memory", wantIn: []string{"12884901888"}},
		{name: "memory probe error", mod: func(p *fakeProbe) { p.memErr = errors.New("boom") }, wantCheck: "memory", wantIn: []string{"boom"}},
		{name: "k3s wrong version", mod: func(p *fakeProbe) { p.k3sInstall, p.k3sVer = true, "v1.30.0+k3s1" }, wantCheck: "k3s", wantIn: []string{"v1.30.0+k3s1", testPin}},
		{name: "servicelb enabled", mod: func(p *fakeProbe) { p.k3sInstall, p.k3sVer = true, testPin }, wantCheck: "k3s", wantIn: []string{"--disable=servicelb"}},
		{name: "k3s correct", mod: func(p *fakeProbe) { p.k3sInstall, p.k3sVer, p.servicelb = true, testPin, true }},
		// ruling 2026-11-26: NPM_TASKFORCESH_TOKEN is not used, so its absence is not a problem
		{name: "taskforce token not required", mod: func(p *fakeProbe) { p.env = map[string]string{"NPM_TOKEN": "x"} }},
		{
			name: "no npm token and no gh token",
			mod: func(p *fakeProbe) {
				p.env, p.ghToken = map[string]string{"OTHER_VAR": "planted-value"}, errors.New("not logged in")
			},
			wantCheck: "build-secrets", wantIn: []string{"NPM_TOKEN"}, wantNotIn: []string{"planted-value", "TASKFORCE"},
		},
		{name: "missing certutil", mod: func(p *fakeProbe) { p.cmds["certutil"] = false }, wantCheck: "tools", wantIn: []string{"libnss3-tools"}},
		{name: "gh not logged in", mod: func(p *fakeProbe) { p.ghStatus = errors.New("exit 1: no accounts") }, wantCheck: "tools", wantIn: []string{"gh auth login", "no accounts"}},
		// re-running up on an installed devbox must not demand the fresh-install 30 GiB its own images already use
		{name: "installed devbox re-run with 26 GiB free", mod: func(p *fakeProbe) {
			p.k3sInstall, p.k3sVer, p.servicelb, p.disk = true, testPin, true, 26135412736
		}},
		{name: "installed devbox below resume floor", mod: func(p *fakeProbe) {
			p.k3sInstall, p.k3sVer, p.servicelb, p.disk = true, testPin, true, DefaultRequirements.MinDiskResumeBytes-1
		}, wantCheck: "disk", wantIn: []string{"10737418240"}},
		{name: "fresh host with 26 GiB free", mod: func(p *fakeProbe) { p.disk = 26135412736 }, wantCheck: "disk", wantIn: []string{"32212254720"}},
		{name: "disk probe error", mod: func(p *fakeProbe) { p.diskE = errors.New("statfs boom") }, wantCheck: "disk", wantIn: []string{"statfs boom"}},
		{name: "k3s probe error", mod: func(p *fakeProbe) { p.k3sErr = errors.New("exec format error") }, wantCheck: "k3s", wantIn: []string{"exec format error"}},
		{name: "k3s config parse error", mod: func(p *fakeProbe) { p.k3sInstall, p.k3sVer, p.cfgErr = true, testPin, errors.New("parse yaml: bad") }, wantCheck: "k3s", wantIn: []string{"parse yaml: bad"}},
		{name: "sudo prompts", mod: func(p *fakeProbe) { p.sudo = false }, wantCheck: "sudo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := okProbe()
			tc.mod(&p)
			got := Preflight(p, DefaultRequirements, testPin, ProfileCore)
			if tc.wantCheck == "" {
				if len(got) != 0 {
					t.Fatalf("want no problems, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Check != tc.wantCheck {
				t.Fatalf("want one %q problem, got %+v", tc.wantCheck, got)
			}
			text := got[0].Detail + " " + got[0].Fix
			for _, s := range tc.wantIn {
				if !strings.Contains(text, s) {
					t.Errorf("problem %q missing %q", text, s)
				}
			}
			for _, s := range tc.wantNotIn {
				if strings.Contains(text, s) {
					t.Errorf("problem %q leaks %q", text, s)
				}
			}
		})
	}
}

func TestPreflight_ReturnsAllInOrder(t *testing.T) {
	p := okProbe()
	p.os, p.mem = "darwin", 1
	p.cmds["curl"] = false
	got := Preflight(p, DefaultRequirements, testPin, ProfileCore)
	var checks []string
	for _, pr := range got {
		checks = append(checks, pr.Check)
	}
	if strings.Join(checks, ",") != "os,memory,tools" {
		t.Fatalf("order = %v", checks)
	}
}

func TestConfigDisablesServicelb(t *testing.T) {
	cases := map[string]bool{
		"disable:\n  - traefik\n  - servicelb\n":    true,
		"disable: [traefik, servicelb]\n":           true,
		"disable: [\n  traefik,\n  servicelb,\n]\n": true,
		"disable:\n  - \"servicelb\"  # lb\n":       true,
		"disable: servicelb\n":                      true,
		"disable: traefik,servicelb\n":              true,
		"disable:\n  - traefik\nother: x\n":         false,
		"# disable: servicelb\n":                    false,
		"other:\n  - servicelb\n":                   false,
		"":                                          false,
	}
	for in, want := range cases {
		got, err := configDisablesServicelb([]byte(in))
		if err != nil || got != want {
			t.Errorf("%q: got %v err %v want %v", in, got, err, want)
		}
	}
}

func TestConfigDisablesServicelb_Errors(t *testing.T) {
	for _, in := range []string{"disable: [unclosed\n", "disable:\n  - 1\n", "disable: {a: b}\n"} {
		if _, err := configDisablesServicelb([]byte(in)); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

// Evidence: a web build OOM-killed buildkitd on a 12 GiB host next to the full
// env, so full needs 16 GiB while core passes on 12.
func TestPreflight_MemoryFloorPerProfile(t *testing.T) {
	p := okProbe()
	p.mem = 12 << 30
	if got := Preflight(p, DefaultRequirements, testPin, ProfileCore); len(got) != 0 {
		t.Errorf("12 GiB host must pass core: %+v", got)
	}
	got := Preflight(p, DefaultRequirements, testPin, ProfileFull)
	if len(got) != 1 || got[0].Check != "memory" || !strings.Contains(got[0].Detail, "17179869184") || !strings.Contains(got[0].Detail, "profile full") {
		t.Fatalf("12 GiB host must fail full naming 16 GiB: %+v", got)
	}
	p.mem = 16 << 30
	if got := Preflight(p, DefaultRequirements, testPin, ProfileFull); len(got) != 0 {
		t.Errorf("16 GiB host must pass full: %+v", got)
	}
}
