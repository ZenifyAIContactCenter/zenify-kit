package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
)

// Repos that install private packages from GitHub Packages reference ${NPM_TOKEN} in their
// .npmrc. `zenify up` sets that variable in the user's shell profile from `gh auth token`, so
// the token itself is never written to disk or printed by the kit.
const (
	npmTokenMarker     = "# zenify-npm-token" //nolint:gosec // G101 -- a profile marker comment, not a credential
	npmTokenLine       = "export NPM_TOKEN=\"${NPM_TOKEN:-$(gh auth token 2>/dev/null)}\" " + npmTokenMarker
	npmTokenScope      = "read:packages"
	npmTokenRefreshCmd = "gh auth refresh -h github.com -s read:packages" //nolint:gosec // G101 -- a command the user runs, not a credential
	githubPackagesHost = "npm.pkg.github.com"
)

// needsGitHubPackagesToken reports whether any <dir>/.npmrc has a non-comment line that points
// at GitHub Packages and reads its token from ${NPM_TOKEN}, e.g.
// //npm.pkg.github.com/:_authToken=${NPM_TOKEN}.
func needsGitHubPackagesToken(repoDirs []string) bool {
	for _, d := range repoDirs {
		b, err := os.ReadFile(filepath.Join(d, ".npmrc")) //nolint:gosec // G304 -- path built from manifest repo dirs
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			l := strings.TrimSpace(line)
			if strings.HasPrefix(l, "#") || strings.HasPrefix(l, ";") {
				continue
			}
			if strings.Contains(l, githubPackagesHost) && strings.Contains(l, "${NPM_TOKEN}") {
				return true
			}
		}
	}
	return false
}

// repoDirsFor lists the absolute directories of the manifest repos of workspace ws.
func repoDirsFor(ws string) ([]string, error) {
	rels, err := manifestRepoPaths(ws)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(rels))
	for _, r := range rels {
		if !filepath.IsAbs(r) {
			r = filepath.Join(ws, r)
		}
		dirs = append(dirs, r)
	}
	return dirs, nil
}

// shellProfilePath picks the profile the same way scripts/install.sh does: zsh → ~/.zshrc,
// bash → ~/.bashrc when it exists else ~/.profile, anything else → "".
func shellProfilePath(shell, home string, exists func(string) bool) string {
	switch {
	case strings.HasSuffix(shell, "/zsh"):
		return filepath.Join(home, ".zshrc")
	case strings.HasSuffix(shell, "/bash"):
		if p := filepath.Join(home, ".bashrc"); exists(p) {
			return p
		}
		return filepath.Join(home, ".profile")
	}
	return ""
}

type profileState int

const (
	profileMissing      profileState = iota // neither our line nor a user-set NPM_TOKEN
	profileHasMarker                        // our marker line is present
	profileHasUserToken                     // the user sets NPM_TOKEN themselves
)

// readProfileState classifies a shell profile's content. Only non-comment lines that set
// NPM_TOKEN count; such a line carrying the marker is ours, any other is the user's.
func readProfileState(content string) profileState {
	state := profileMissing
	for _, line := range strings.Split(content, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Fields(l)
		if len(f) > 1 && f[0] == "export" {
			f = f[1:]
		}
		if len(f) == 0 || !strings.HasPrefix(f[0], "NPM_TOKEN=") {
			continue
		}
		if strings.Contains(l, npmTokenMarker) {
			return profileHasMarker
		}
		state = profileHasUserToken
	}
	return state
}

// npmTokenInteractive reports whether zenify up may launch `gh auth refresh`: both stdin and
// stdout are terminals and --non-interactive was not given.
func npmTokenInteractive(nonInteractive, stdinTTY, stdoutTTY bool) bool {
	return !nonInteractive && stdinTTY && stdoutTTY
}

// npmTokenSourceHint is the advice for making a freshly written profile line take effect.
func npmTokenSourceHint(path string) string {
	return fmt.Sprintf("run \"source %s\" or open a new terminal before npm install", path)
}

// ensureNPMTokenProfile appends npmTokenLine to path when the profile is profileMissing.
func ensureNPMTokenProfile(path string) (bool, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- the user's own shell profile
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if readProfileState(string(b)) != profileMissing {
		return false, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G302/G304 -- shell profile, user-readable like install.sh writes it
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString("\n" + npmTokenLine + "\n"); err != nil {
		_ = f.Close()
		return false, err
	}
	return true, f.Close()
}

// ensureNPMToken makes NPM_TOKEN available to new shells when a workspace repo installs from
// GitHub Packages. Silent when no repo needs it. Callers treat a returned error as a warning.
func ensureNPMToken(w io.Writer, repoDirs []string, gh ghx.Runner, interactive bool, refresh func() error) error {
	if !needsGitHubPackagesToken(repoDirs) {
		return nil
	}
	return setupNPMToken(w, gh, interactive, refresh)
}

// setupNPMToken is ensureNPMToken without the workspace check: it judges the active gh account
// and writes the profile line regardless of which repos exist.
func setupNPMToken(w io.Writer, gh ghx.Runner, interactive bool, refresh func() error) error {
	auth, err := ghx.CheckActiveAuth(gh, "github.com")
	if err != nil {
		return fmt.Errorf("npm token: gh auth status: %w", err)
	}
	if !auth.LoggedIn {
		fmt.Fprintln(w, "npm token: gh is not logged in; run `gh auth login`, then `zenify up` again")
		return nil
	}
	// gh reports no scopes line when the token comes from GH_TOKEN/GITHUB_TOKEN: the scope is
	// unknowable, so never refresh in that case.
	scopesKnown := len(auth.Scopes) > 0
	scopeMissing := scopesKnown && !auth.HasScopes(npmTokenScope)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return fmt.Errorf("npm token: home dir: %w", err)
	}
	path := shellProfilePath(os.Getenv("SHELL"), home, func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	})
	if path == "" {
		if scopeMissing {
			fmt.Fprintf(w, "npm token: gh lacks %s; run `%s`\n", npmTokenScope, npmTokenRefreshCmd)
		}
		fmt.Fprintf(w, "npm token: unknown shell; add this line to your shell profile: %s\n", npmTokenLine)
		return nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- the user's own shell profile
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("npm token: read %s: %w", path, err)
	}
	state := readProfileState(string(b))
	if state == profileHasUserToken {
		fmt.Fprintf(w, "npm token: %s already sets NPM_TOKEN; left unchanged\n", path)
		return nil
	}
	if scopeMissing {
		if !interactive {
			fmt.Fprintf(w, "npm token: gh lacks %s; run `%s`, then `zenify up` again\n", npmTokenScope, npmTokenRefreshCmd)
			return nil
		}
		if err := refresh(); err != nil {
			return fmt.Errorf("npm token: %s: %w", npmTokenRefreshCmd, err)
		}
	}
	if !scopesKnown {
		fmt.Fprintf(w, "npm token: could not determine gh token scopes; npm install needs %s\n", npmTokenScope)
	}
	changed, err := ensureNPMTokenProfile(path)
	if err != nil {
		return fmt.Errorf("npm token: write %s: %w", path, err)
	}
	switch {
	case changed:
		fmt.Fprintf(w, "npm token: added NPM_TOKEN to %s — %s\n", path, npmTokenSourceHint(path))
	case state == profileHasMarker && os.Getenv("NPM_TOKEN") == "":
		fmt.Fprintf(w, "npm token: %s sets NPM_TOKEN but this shell lacks it — %s\n", path, npmTokenSourceHint(path))
	}
	return nil
}

// currentShellProfile returns the content of the user's shell profile and whether the shell is
// one zenify up knows (zsh/bash). An unreadable or absent profile reads as "".
func currentShellProfile() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	path := shellProfilePath(os.Getenv("SHELL"), home, func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	})
	if path == "" {
		return "", false
	}
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- the user's own shell profile
	if err != nil {
		return "", true
	}
	return string(b), true
}

// workspaceRepoDirs lists the manifest repo directories of the current workspace.
func workspaceRepoDirs() []string {
	cwd, _ := os.Getwd()
	ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir)
	if !ok {
		return nil
	}
	dirs, _ := repoDirsFor(ws)
	return dirs
}

// npmTokenCheck is the doctor check for FR-6: when a workspace repo installs from GitHub
// Packages, gh must carry read:packages and NPM_TOKEN must reach new shells (our profile line,
// a user-set export, or the current environment). profile returns the shell profile content and
// whether the shell is known. The token value is never read into the detail.
func npmTokenCheck(repoDirs func() []string, gh ghx.Runner, profile func() (string, bool), fix func() (bool, string)) Check {
	return Check{
		Name: "npm token (GitHub Packages)",
		Run: func() (bool, string) {
			if !needsGitHubPackagesToken(repoDirs()) {
				return true, "n/a — no repo uses GitHub Packages"
			}
			return npmTokenStatus(gh, profile)
		},
		Fix: fix,
	}
}

// npmTokenCheckTimeout bounds the gh auth status call so a hung gh cannot stall doctor.
var npmTokenCheckTimeout = 10 * time.Second

// npmTokenStatus judges the active gh account and NPM_TOKEN reachability, without the workspace
// check. The token value is never read into the detail.
func npmTokenStatus(gh ghx.Runner, profile func() (string, bool)) (bool, string) {
	type authResult struct {
		auth ghx.Auth
		err  error
	}
	ch := make(chan authResult, 1)
	go func() { a, err := ghx.CheckActiveAuth(gh, "github.com"); ch <- authResult{a, err} }()
	var auth ghx.Auth
	var err error
	select {
	case r := <-ch:
		auth, err = r.auth, r.err
	case <-time.After(npmTokenCheckTimeout):
		return false, "gh auth status timed out"
	}
	if err != nil || !auth.LoggedIn {
		return false, "gh not logged in — run `gh auth login`"
	}
	// No scopes line (token from GH_TOKEN/GITHUB_TOKEN): the scope is unknowable, so
	// judge only by NPM_TOKEN below.
	okDetail := "ok"
	if len(auth.Scopes) == 0 {
		okDetail = "ok (gh token scopes unknown)"
	} else if !auth.HasScopes(npmTokenScope) {
		return false, "gh lacks " + npmTokenScope + " — run `" + npmTokenRefreshCmd + "`"
	}
	if os.Getenv("NPM_TOKEN") != "" {
		return true, okDetail
	}
	content, known := profile()
	if !known {
		return false, "NPM_TOKEN not set — add `" + npmTokenLine + "` to your shell profile"
	}
	if readProfileState(content) == profileMissing {
		return false, "NPM_TOKEN not set — run `zenify up`"
	}
	return true, okDetail
}
