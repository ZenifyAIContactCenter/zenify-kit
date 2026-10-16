package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// recGH is a hermetic gh fake that records every invocation.
type recGH struct {
	version, list string
	failAll       bool
	installErr    error
	calls         []string
}

func (r *recGH) Run(args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	r.calls = append(r.calls, cmd)
	if r.failAll {
		return nil, errors.New("exec: gh not found")
	}
	switch {
	case cmd == "--version":
		return []byte(r.version), nil
	case cmd == "extension list":
		return []byte(r.list), nil
	case cmd == ghStackInstallCmd[len("gh "):]:
		return nil, r.installErr
	}
	return nil, errors.New("unexpected: " + cmd)
}

func (r *recGH) installed() bool {
	for _, c := range r.calls {
		if strings.HasPrefix(c, "extension install") {
			return true
		}
	}
	return false
}

const (
	verOK  = "gh version 2.100.0 (2026-09-03)\nhttps://github.com/cli/cli/releases/tag/v2.100.0\n"
	extOK  = "gh stack\tgithub/gh-stack\tv0.2.1\n"
	extOth = "gh copilot\tgithub/gh-copilot\tv1.0.0\n"
)

func TestGHStackStatus(t *testing.T) {
	tests := []struct {
		name    string
		g       *recGH
		ok      bool
		contain string
	}{
		{"ok", &recGH{version: verOK, list: extOK}, true, "gh-stack v0.2.1"},
		{"old gh", &recGH{version: "gh version 2.89.1 (2026-01-01)\n", list: extOK}, false, "2.90.0"},
		{"numeric not string compare", &recGH{version: "gh version 2.9.0 (2024-01-01)\n", list: extOK}, false, "2.90.0"},
		{"extension missing", &recGH{version: verOK, list: extOth}, false, "gh extension install github/gh-stack"},
		{"empty list", &recGH{version: verOK}, false, "gh extension install github/gh-stack"},
		{"gh fails", &recGH{failAll: true}, false, "gh not available"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, detail := ghStackStatus(tc.g)
			if ok != tc.ok || !strings.Contains(detail, tc.contain) {
				t.Errorf("ok=%v detail=%q, want ok=%v containing %q", ok, detail, tc.ok, tc.contain)
			}
		})
	}
}

func TestEnsureGHStack(t *testing.T) {
	t.Run("installed is silent", func(t *testing.T) {
		g := &recGH{version: verOK, list: extOK}
		var w, e bytes.Buffer
		ensureGHStack(g, &w, &e)
		if g.installed() || w.Len() != 0 || e.Len() != 0 {
			t.Errorf("calls=%v w=%q e=%q", g.calls, w.String(), e.String())
		}
	})
	t.Run("missing installs", func(t *testing.T) {
		g := &recGH{version: verOK}
		var w, e bytes.Buffer
		ensureGHStack(g, &w, &e)
		if got := g.calls[len(g.calls)-1]; got != "extension install github/gh-stack" {
			t.Errorf("last call = %q", got)
		}
		if !strings.Contains(w.String(), "gh-stack: installed") {
			t.Errorf("w=%q", w.String())
		}
	})
	t.Run("install error warns", func(t *testing.T) {
		g := &recGH{version: verOK, installErr: errors.New("boom")}
		var w, e bytes.Buffer
		ensureGHStack(g, &w, &e)
		if !strings.Contains(e.String(), "warning: gh-stack install") {
			t.Errorf("e=%q", e.String())
		}
	})
	t.Run("gh missing warns", func(t *testing.T) {
		g := &recGH{failAll: true}
		var w, e bytes.Buffer
		ensureGHStack(g, &w, &e)
		if g.installed() || !strings.Contains(e.String(), "warning: gh-stack: gh not available") {
			t.Errorf("calls=%v e=%q", g.calls, e.String())
		}
	})
}
