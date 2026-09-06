package tui

import (
	"errors"
	"os/exec"
	"strings"
)

var errNotFound = errors.New("gh not found")

func detectGH() error { return detectGHWith(exec.LookPath) }

func detectGHWith(lookPath func(string) (string, error)) error {
	if _, err := lookPath("gh"); err != nil {
		return errors.New("GitHub CLI (gh) not found — install: https://cli.github.com then re-run `zenify up`")
	}
	return nil
}

// parseAuthStatus reads `gh auth status` text output.
func parseAuthStatus(out string) (account string, ok bool) {
	if strings.Contains(out, "not logged in") || strings.Contains(out, "not logged into") {
		return "", false
	}
	// "Logged in to github.com account <name>"
	if i := strings.Index(out, "account "); i >= 0 {
		rest := out[i+len("account "):]
		f := strings.Fields(rest)
		if len(f) > 0 {
			return f[0], true
		}
	}
	if strings.Contains(out, "Logged in") {
		return "", true
	}
	return "", false
}

func ghAuthStatus() (string, bool) {
	out, _ := exec.Command("gh", "auth", "status").CombinedOutput()
	return parseAuthStatus(string(out))
}

func loginCmd() *exec.Cmd { return exec.Command("gh", "auth", "login", "--web") }
