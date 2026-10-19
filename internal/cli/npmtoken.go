package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
)

// Repos that install private packages from GitHub Packages reference ${NPM_TOKEN} in their
// .npmrc. `zenify up` sets that variable in the user's shell profile from `gh auth token`, so
// the token itself is never written to disk or printed by the kit.
const (
	npmTokenMarker     = "# zenify-npm-token" //nolint:gosec // G101 -- a profile marker comment, not a credential
	npmTokenLine       = "export NPM_TOKEN=\"$(gh auth token 2>/dev/null)\" " + npmTokenMarker
	npmTokenScope      = "read:packages"
	npmTokenRefreshCmd = "gh auth refresh -h github.com -s read:packages" //nolint:gosec // G101 -- a command the user runs, not a credential
	githubPackagesHost = "npm.pkg.github.com"
)

// needsGitHubPackagesToken reports whether any <dir>/.npmrc points at GitHub Packages and
// reads its token from ${NPM_TOKEN}.
func needsGitHubPackagesToken(repoDirs []string) bool {
	for _, d := range repoDirs {
		b, err := os.ReadFile(filepath.Join(d, ".npmrc")) //nolint:gosec // G304 -- path built from manifest repo dirs
		if err != nil {
			continue
		}
		s := string(b)
		if strings.Contains(s, githubPackagesHost) && strings.Contains(s, "${NPM_TOKEN}") {
			return true
		}
	}
	return false
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

// readProfileState classifies a shell profile's content.
func readProfileState(content string) profileState {
	if strings.Contains(content, npmTokenMarker) {
		return profileHasMarker
	}
	for _, line := range strings.Split(content, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "#") {
			continue
		}
		l = strings.TrimSpace(strings.TrimPrefix(l, "export "))
		if strings.HasPrefix(l, "NPM_TOKEN=") {
			return profileHasUserToken
		}
	}
	return profileMissing
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
	auth, err := ghx.CheckAuth(gh)
	if err != nil {
		return fmt.Errorf("npm token: gh auth status: %w", err)
	}
	if !auth.LoggedIn {
		fmt.Fprintln(w, "npm token: gh is not logged in; run `gh auth login`, then `zenify up` again")
		return nil
	}
	if !auth.HasScopes(npmTokenScope) {
		if !interactive {
			fmt.Fprintf(w, "npm token: gh lacks %s; run `%s`, then `zenify up` again\n", npmTokenScope, npmTokenRefreshCmd)
			return nil
		}
		if err := refresh(); err != nil {
			return fmt.Errorf("npm token: %s: %w", npmTokenRefreshCmd, err)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return fmt.Errorf("npm token: home dir: %w", err)
	}
	path := shellProfilePath(os.Getenv("SHELL"), home, func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	})
	if path == "" {
		fmt.Fprintf(w, "npm token: unknown shell; add this line to your shell profile: %s\n", npmTokenLine)
		return nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- the user's own shell profile
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("npm token: read %s: %w", path, err)
	}
	if readProfileState(string(b)) == profileHasUserToken {
		fmt.Fprintf(w, "npm token: %s already sets NPM_TOKEN; left unchanged\n", path)
		return nil
	}
	changed, err := ensureNPMTokenProfile(path)
	if err != nil {
		return fmt.Errorf("npm token: write %s: %w", path, err)
	}
	if changed {
		fmt.Fprintf(w, "npm token: added NPM_TOKEN to %s (takes effect in new shells)\n", path)
	}
	return nil
}
