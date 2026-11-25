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
	k3sVer     string
	k3sInstall bool
	servicelb  bool
	env        map[string]string
	keys       map[string]bool
	cmds       map[string]bool
	ghLogged   bool
	ghTokenOK  bool
}

func (f fakeProbe) GOOS() string                         { return f.os }
func (f fakeProbe) HasSystemd() bool                     { return f.systemd }
func (f fakeProbe) CanSudo() bool                        { return f.sudo }
func (f fakeProbe) MemTotalBytes() (uint64, error)       { return f.mem, f.memErr }
func (f fakeProbe) DiskFreeBytes(string) (uint64, error) { return f.disk, nil }
func (f fakeProbe) InstalledK3sVersion() (string, bool)  { return f.k3sVer, f.k3sInstall }
func (f fakeProbe) K3sServicelbDisabled() (bool, bool)   { return f.servicelb, f.k3sInstall }
func (f fakeProbe) EnvSet(k string) bool                 { return f.env[k] != "" }
func (f fakeProbe) HasCommand(n string) bool             { return f.cmds[n] }
func (f fakeProbe) GhLoggedIn() bool                     { return f.ghLogged }
func (f fakeProbe) GhTokenAvailable() bool               { return f.ghTokenOK }
func (f fakeProbe) LookupKey(k string) bool              { return f.keys[k] || f.env[k] != "" }

func okProbe() fakeProbe {
	return fakeProbe{
		os: "linux", systemd: true, sudo: true,
		mem: DefaultRequirements.MinMemBytes, disk: DefaultRequirements.MinDiskBytes,
		env:      map[string]string{"NPM_TOKEN": "x"},
		keys:     map[string]bool{"NPM_TASKFORCESH_TOKEN": true},
		cmds:     map[string]bool{"curl": true, "git": true, "gh": true, "certutil": true},
		ghLogged: true, ghTokenOK: true,
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
		{name: "low memory", mod: func(p *fakeProbe) { p.mem = 1000 }, wantCheck: "memory", wantIn: []string{"6442450944", "1000"}},
		{name: "memory probe error", mod: func(p *fakeProbe) { p.memErr = errors.New("boom") }, wantCheck: "memory", wantIn: []string{"boom"}},
		{name: "k3s wrong version", mod: func(p *fakeProbe) { p.k3sInstall, p.k3sVer = true, "v1.30.0+k3s1" }, wantCheck: "k3s", wantIn: []string{"v1.30.0+k3s1", testPin}},
		{name: "servicelb enabled", mod: func(p *fakeProbe) { p.k3sInstall, p.k3sVer = true, testPin }, wantCheck: "k3s", wantIn: []string{"--disable=servicelb"}},
		{name: "k3s correct", mod: func(p *fakeProbe) { p.k3sInstall, p.k3sVer, p.servicelb = true, testPin, true }},
		{
			name: "missing taskforce token",
			mod: func(p *fakeProbe) {
				p.keys = nil
				p.env = map[string]string{"NPM_TOKEN": "x", "OTHER_SECRET": "s3cr3t-planted"}
			},
			wantCheck: "build-secrets", wantIn: []string{"NPM_TASKFORCESH_TOKEN"}, wantNotIn: []string{"s3cr3t-planted"},
		},
		{name: "no npm token and no gh token", mod: func(p *fakeProbe) { p.env, p.ghTokenOK = nil, false }, wantCheck: "build-secrets", wantIn: []string{"NPM_TOKEN"}},
		{name: "missing certutil", mod: func(p *fakeProbe) { p.cmds["certutil"] = false }, wantCheck: "tools", wantIn: []string{"libnss3-tools"}},
		{name: "gh not logged in", mod: func(p *fakeProbe) { p.ghLogged = false }, wantCheck: "tools", wantIn: []string{"gh auth login"}},
		{name: "sudo prompts", mod: func(p *fakeProbe) { p.sudo = false }, wantCheck: "sudo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := okProbe()
			tc.mod(&p)
			got := Preflight(p, DefaultRequirements, testPin)
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
	got := Preflight(p, DefaultRequirements, testPin)
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
		"disable:\n  - traefik\n  - servicelb\n": true,
		"disable: [traefik, servicelb]\n":        true,
		"disable: servicelb\n":                   true,
		"disable:\n  - traefik\nother: x\n":      false,
		"other:\n  - servicelb\n":                false,
		"":                                       false,
	}
	for in, want := range cases {
		if got := configDisablesServicelb(in); got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}
