package tui

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/huh"
)

var errNotFound = errors.New("gh not found")

func detectGH() error { return detectGHWith(exec.LookPath) }

func detectGHWith(lookPath func(string) (string, error)) error {
	if _, err := lookPath("gh"); err != nil {
		return errGHGuide // single source of the gh-missing guide message
	}
	return nil
}

func detectGit() error { return detectGitWith(exec.LookPath) }

func detectGitWith(lookPath func(string) (string, error)) error {
	if _, err := lookPath("git"); err != nil {
		return errors.New("git not found — install Xcode Command Line Tools (`xcode-select --install`) or git, then re-run `zenify up`")
	}
	return nil
}

// errGHGuide is the guide-only error when gh is missing (keeps the P1 message).
var errGHGuide = errors.New("GitHub CLI (gh) not found — install: https://cli.github.com then re-run `zenify up`")

// offerInstallGH runs when gh is missing in interactive mode: asks to install via Homebrew,
// and if brew is present and the user agrees, runs InstallRunner then re-detects. Every other
// path (no brew / declined) returns errGHGuide.
func offerInstallGH(cfg OnboardConfig) error {
	lookPath := cfg.lookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if _, err := lookPath("brew"); err != nil {
		return errGHGuide // no brew → can't auto-install
	}

	confirm := cfg.ConfirmFn
	if confirm == nil {
		confirm = huhConfirm
	}
	ok, err := confirm("gh chưa có. Cài gh qua Homebrew ngay?") //znf:allow-lang
	if err != nil {
		return err
	}
	if !ok {
		return errGHGuide
	}

	run := cfg.InstallRunner
	if run == nil {
		run = brewInstall
	}
	if err := run("gh"); err != nil {
		return err
	}

	detect := cfg.DetectGHFn
	if detect == nil {
		detect = detectGH
	}
	return detect() // re-detect after install
}

// huhConfirm is the real ConfirmFn, using huh.
func huhConfirm(prompt string) (bool, error) {
	v := false
	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(prompt).Value(&v),
	)).Run()
	return v, err
}

// brewInstall is the real InstallRunner: runs `brew install <tool>` with stdio attached to
// the terminal so the user sees progress.
func brewInstall(tool string) error {
	cmd := exec.Command("brew", "install", tool) //nolint:gosec // G204 -- fixed 'brew install'; tool is an internal constant, not user shell input
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type authState int

const (
	authLoggedIn authState = iota
	authLoggedOut
	authUnreachable
)

// networkMarkers are the strings that appear in `gh auth status` output when
// the machine can't reach GitHub (distinguishing it from not-logged-in).
var networkMarkers = []string{
	"could not connect",
	"dial tcp",
	"no such host",
	"connection refused",
	"i/o timeout",
	"context deadline exceeded",
	"client.timeout exceeded",
	"network is unreachable",
}

// parseAuthState classifies `gh auth status` output into 3 states. err is
// the process error from `gh auth status` (non-nil when exit≠0). offline is
// recognized by err≠nil PLUS a network marker in the output; without a marker it's
// treated as not-logged-in (safe: falls through to the login flow).
func parseAuthState(out string, err error) (account string, state authState) {
	lower := strings.ToLower(out)
	if err != nil {
		for _, m := range networkMarkers {
			if strings.Contains(lower, m) {
				return "", authUnreachable
			}
		}
	}
	if strings.Contains(out, "not logged in") || strings.Contains(out, "not logged into") {
		return "", authLoggedOut
	}
	// "Logged in to github.com account <name>"
	if i := strings.Index(out, "account "); i >= 0 {
		rest := out[i+len("account "):]
		if f := strings.Fields(rest); len(f) > 0 {
			return f[0], authLoggedIn
		}
	}
	if strings.Contains(out, "Logged in") {
		return "", authLoggedIn
	}
	return "", authLoggedOut
}

func ghAuthStatus() (string, authState) {
	out, err := exec.Command("gh", "auth", "status").CombinedOutput()
	return parseAuthState(string(out), err)
}

func loginCmd() *exec.Cmd { return exec.Command("gh", "auth", "login", "--web") }
