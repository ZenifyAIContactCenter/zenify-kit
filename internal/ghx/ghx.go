// Package ghx is a thin adapter over the `gh` CLI: auth status and repo list.
// It never returns or logs the auth token. Read-only.
package ghx

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Runner abstracts invoking `gh` so callers can inject a fake in tests.
type Runner interface {
	Run(args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(args ...string) ([]byte, error) {
	return exec.Command("gh", args...).Output()
}

// ExecRunner returns the default Runner that shells to `gh`.
func ExecRunner() Runner { return execRunner{} }

// Auth is the parsed, token-free result of `gh auth status`.
type Auth struct {
	LoggedIn bool
	Account  string
	Protocol string
	Scopes   []string
}

// HasScopes reports whether every requested scope is present.
func (a Auth) HasScopes(need ...string) bool {
	have := map[string]bool{}
	for _, s := range a.Scopes {
		have[s] = true
	}
	for _, n := range need {
		if !have[n] {
			return false
		}
	}
	return true
}

// CheckAuth parses `gh auth status`. A logged-out state (gh exits non-zero) is
// reported as Auth{LoggedIn:false}, not an error. The token line is ignored.
func CheckAuth(r Runner) (Auth, error) {
	out, err := r.Run("auth", "status")
	text := string(out)
	a := Auth{}
	if strings.Contains(text, "Logged in to") {
		a.LoggedIn = true
	}
	if !a.LoggedIn {
		return a, nil // logged-out; gh's non-zero exit is expected, not fatal
	}
	_ = err
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.Contains(line, "Logged in to") && strings.Contains(line, "account"):
			// "✓ Logged in to github.com account natepxn (keyring)"
			if i := strings.Index(line, "account "); i >= 0 {
				rest := strings.TrimSpace(line[i+len("account "):])
				a.Account = strings.Fields(rest)[0]
			}
		case strings.HasPrefix(line, "- Git operations protocol:"):
			a.Protocol = strings.TrimSpace(strings.TrimPrefix(line, "- Git operations protocol:"))
		case strings.HasPrefix(line, "- Token scopes:"):
			raw := strings.TrimSpace(strings.TrimPrefix(line, "- Token scopes:"))
			for _, s := range strings.Split(raw, ",") {
				s = strings.TrimSpace(s)
				s = strings.Trim(s, "'\"")
				if s != "" {
					a.Scopes = append(a.Scopes, s)
				}
			}
		}
	}
	return a, nil
}

// RemoteRepo is one repo from `gh repo list --json ...`.
type RemoteRepo struct {
	Name             string `json:"name"`
	SSHURL           string `json:"sshUrl"`
	ViewerPermission string `json:"viewerPermission"`
	IsArchived       bool   `json:"isArchived"`
}

// HasAccess reports whether the viewer can access the repo at all.
func (r RemoteRepo) HasAccess() bool {
	return r.ViewerPermission != "" && r.ViewerPermission != "NONE"
}

// ListRepos returns every repo the viewer can see in org, with per-repo access.
func ListRepos(r Runner, org string) ([]RemoteRepo, error) {
	out, err := r.Run("repo", "list", org, "--json", "name,viewerPermission,sshUrl,isArchived", "--limit", "1000")
	if err != nil {
		return nil, fmt.Errorf("gh repo list: %w", err)
	}
	var repos []RemoteRepo
	if err := json.Unmarshal(out, &repos); err != nil {
		return nil, fmt.Errorf("parse gh repo list: %w", err)
	}
	return repos, nil
}
