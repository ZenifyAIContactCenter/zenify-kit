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
		f := strings.Fields(l)
		if len(f) > 1 && f[0] == "export" {
			f = f[1:]
		}
		if len(f) > 0 && strings.HasPrefix(f[0], "NPM_TOKEN=") {
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
	if !auth.HasScopes(npmTokenScope) {
		if !interactive {
			fmt.Fprintf(w, "npm token: gh lacks %s; run `%s`, then `zenify up` again\n", npmTokenScope, npmTokenRefreshCmd)
			return nil
		}
		if err := refresh(); err != nil {
			return fmt.Errorf("npm token: %s: %w", npmTokenRefreshCmd, err)
		}
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

// currentShellProfile returns the content of the user's shell profile, "" when the shell is
// unknown or the file is unreadable.
func currentShellProfile() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	path := shellProfilePath(os.Getenv("SHELL"), home, func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	})
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- the user's own shell profile
	if err != nil {
		return ""
	}
	return string(b)
}

// workspaceRepoDirs lists the manifest repo directories of the current workspace.
func workspaceRepoDirs() []string {
	cwd, _ := os.Getwd()
	ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir)
	if !ok {
		return nil
	}
	rels, err := manifestRepoPaths(ws)
	if err != nil {
		return nil
	}
	dirs := make([]string, 0, len(rels))
	for _, r := range rels {
		if !filepath.IsAbs(r) {
			r = filepath.Join(ws, r)
		}
		dirs = append(dirs, r)
	}
	return dirs
}

// npmTokenCheck is the doctor check for FR-6: when a workspace repo installs from GitHub
// Packages, gh must carry read:packages and NPM_TOKEN must reach new shells (our profile line,
// a user-set export, or the current environment). The token value is never read into the detail.
func npmTokenCheck(repoDirs func() []string, gh ghx.Runner, profile func() string) Check {
	return Check{
		Name: "npm token (GitHub Packages)",
		Run: func() (bool, string) {
			if !needsGitHubPackagesToken(repoDirs()) {
				return true, "n/a — no repo uses GitHub Packages"
			}
			auth, err := ghx.CheckAuth(gh)
			if err != nil || !auth.LoggedIn {
				return false, "gh not logged in — run `gh auth login`"
			}
			if !auth.HasScopes(npmTokenScope) {
				return false, "gh lacks " + npmTokenScope + " — run `" + npmTokenRefreshCmd + "`"
			}
			if readProfileState(profile()) == profileMissing && os.Getenv("NPM_TOKEN") == "" {
				return false, "NPM_TOKEN not set — run `zenify up`"
			}
			return true, "ok"
		},
	}
}
