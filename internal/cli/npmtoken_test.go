package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	npmTokenTestAuthWithScope = "github.com\n  ✓ Logged in to github.com account dev (keyring)\n  - Git operations protocol: ssh\n  - Token: ghp_FAKE\n  - Token scopes: 'read:org', 'read:packages', 'repo'\n"
	npmTokenTestAuthNoScope   = "github.com\n  ✓ Logged in to github.com account dev (keyring)\n  - Git operations protocol: ssh\n  - Token: ghp_FAKE\n  - Token scopes: 'read:org', 'repo'\n"
	npmTokenTestGHPkgNpmrc    = "@acme:registry=https://npm.pkg.github.com\n//npm.pkg.github.com/:_authToken=${NPM_TOKEN}\n"
	npmTokenTestWantLine      = `export NPM_TOKEN="$(gh auth token 2>/dev/null)" # zenify-npm-token`
)

// npmTokenTestEnv makes a temp HOME with SHELL=zsh and returns (home, zshrc path, repo dirs).
// withGHPkg controls whether the single repo's .npmrc uses GitHub Packages.
func npmTokenTestEnv(t *testing.T, withGHPkg bool) (string, []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	repo := filepath.Join(t.TempDir(), "repo-a")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	npmrc := "registry=https://registry.npmjs.org\n"
	if withGHPkg {
		npmrc = npmTokenTestGHPkgNpmrc
	}
	if err := os.WriteFile(filepath.Join(repo, ".npmrc"), []byte(npmrc), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".zshrc"), []string{repo}
}

func npmTokenTestRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNPMToken_NeedsGitHubPackagesToken(t *testing.T) {
	dir := t.TempDir()
	mk := func(name, npmrc string) string {
		d := filepath.Join(dir, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if npmrc != "" {
			if err := os.WriteFile(filepath.Join(d, ".npmrc"), []byte(npmrc), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return d
	}
	none := mk("none", "")
	public := mk("public", "registry=https://registry.npmjs.org\n")
	hostOnly := mk("hostonly", "@acme:registry=https://npm.pkg.github.com\n")
	ghpkg := mk("ghpkg", npmTokenTestGHPkgNpmrc)
	cases := []struct {
		name string
		dirs []string
		want bool
	}{
		{"no dirs", nil, false},
		{"no npmrc", []string{none}, false},
		{"public registry", []string{public}, false},
		{"host without token var", []string{hostOnly}, false},
		{"github packages", []string{ghpkg}, true},
		{"mixed", []string{none, public, ghpkg}, true},
	}
	for _, c := range cases {
		if got := needsGitHubPackagesToken(c.dirs); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestNPMToken_ShellProfilePath(t *testing.T) {
	home := "/h"
	bashrcExists := func(p string) bool { return p == "/h/.bashrc" }
	never := func(string) bool { return false }
	cases := []struct {
		shell  string
		exists func(string) bool
		want   string
	}{
		{"/bin/zsh", never, "/h/.zshrc"},
		{"/opt/homebrew/bin/zsh", never, "/h/.zshrc"},
		{"/bin/bash", bashrcExists, "/h/.bashrc"},
		{"/bin/bash", never, "/h/.profile"},
		{"/usr/bin/fish", never, ""},
		{"", never, ""},
	}
	for _, c := range cases {
		if got := shellProfilePath(c.shell, home, c.exists); got != c.want {
			t.Errorf("shell %q: got %q want %q", c.shell, got, c.want)
		}
	}
}

func TestNPMToken_ReadProfileState(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    profileState
	}{
		{"empty", "", profileMissing},
		{"path line only", "export PATH='/x/bin':\"$PATH\" # zenify-kit\n", profileMissing},
		{"marker line", "alias ll='ls -l'\n" + npmTokenTestWantLine + "\n", profileHasMarker},
		{"user export", "export NPM_TOKEN=abc\n", profileHasUserToken},
		{"user plain assign", "NPM_TOKEN=abc\n", profileHasUserToken},
		{"commented out", "# export NPM_TOKEN=abc\n", profileMissing},
		{"other variable with same suffix", "MY_NPM_TOKEN=x\n", profileMissing},
		{"export with tab", "export\tNPM_TOKEN=x\n", profileHasUserToken},
	}
	for _, c := range cases {
		if got := readProfileState(c.content); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestNPMToken_LineConstant(t *testing.T) {
	if npmTokenLine != npmTokenTestWantLine {
		t.Errorf("npmTokenLine = %q, want %q", npmTokenLine, npmTokenTestWantLine)
	}
}

func TestNPMToken_EnsureProfileIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".zshrc")
	pre := "export PATH='/x/bin':\"$PATH\" # zenify-kit\n"
	if err := os.WriteFile(p, []byte(pre), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := ensureNPMTokenProfile(p)
	if err != nil || !changed {
		t.Fatalf("first run: changed=%v err=%v", changed, err)
	}
	want := pre + "\n" + npmTokenTestWantLine + "\n"
	if got := npmTokenTestRead(t, p); got != want {
		t.Fatalf("profile = %q, want %q", got, want)
	}
	changed, err = ensureNPMTokenProfile(p)
	if err != nil || changed {
		t.Fatalf("second run: changed=%v err=%v", changed, err)
	}
	if got := npmTokenTestRead(t, p); got != want {
		t.Fatalf("second run modified profile: %q", got)
	}
}

func TestNPMToken_EnsureNPMToken(t *testing.T) {
	cases := []struct {
		name        string
		ghpkg       bool
		auth        string
		interactive bool
		pre         *string // nil = profile file absent
		wantRefresh int
		wantProfile string // exact expected content
		wantOut     []string
		wantNoOut   bool
	}{
		{
			name: "no github packages repo", ghpkg: false, auth: npmTokenTestAuthWithScope,
			wantProfile: "", wantNoOut: true,
		},
		{
			name: "scope present, profile missing", ghpkg: true, auth: npmTokenTestAuthWithScope,
			wantProfile: "\n" + npmTokenTestWantLine + "\n", wantOut: []string{".zshrc"},
		},
		{
			name: "user token kept", ghpkg: true, auth: npmTokenTestAuthWithScope,
			pre: strPtr("export NPM_TOKEN=abc\n"), wantProfile: "export NPM_TOKEN=abc\n",
			wantOut: []string{"NPM_TOKEN", ".zshrc"},
		},
		{
			name: "user token kept, scope missing interactive: no refresh", ghpkg: true, auth: npmTokenTestAuthNoScope, interactive: true,
			pre: strPtr("export NPM_TOKEN=abc\n"), wantRefresh: 0, wantProfile: "export NPM_TOKEN=abc\n",
			wantOut: []string{"NPM_TOKEN", ".zshrc"},
		},
		{
			name: "gh logged out", ghpkg: true, auth: "You are not logged into any GitHub hosts. To log in, run: gh auth login\n", interactive: true,
			wantRefresh: 0, wantProfile: "", wantOut: []string{"gh auth login"},
		},
		{
			name: "scope missing interactive", ghpkg: true, auth: npmTokenTestAuthNoScope, interactive: true,
			wantRefresh: 1, wantProfile: "\n" + npmTokenTestWantLine + "\n", wantOut: []string{".zshrc"},
		},
		{
			name: "scope missing non-interactive", ghpkg: true, auth: npmTokenTestAuthNoScope, interactive: false,
			wantRefresh: 0, wantProfile: "", wantOut: []string{"gh auth refresh -h github.com -s read:packages"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			zshrc, dirs := npmTokenTestEnv(t, c.ghpkg)
			if c.pre != nil {
				if err := os.WriteFile(zshrc, []byte(*c.pre), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			refreshes := 0
			refresh := func() error { refreshes++; return nil }
			var out bytes.Buffer
			if err := ensureNPMToken(&out, dirs, stubGH{auth: []byte(c.auth)}, c.interactive, refresh); err != nil {
				t.Fatalf("err: %v", err)
			}
			if refreshes != c.wantRefresh {
				t.Errorf("refresh calls = %d, want %d", refreshes, c.wantRefresh)
			}
			if got := npmTokenTestRead(t, zshrc); got != c.wantProfile {
				t.Errorf("profile = %q, want %q", got, c.wantProfile)
			}
			s := out.String()
			if c.wantNoOut && s != "" {
				t.Errorf("want no output, got %q", s)
			}
			for _, w := range c.wantOut {
				if !strings.Contains(s, w) {
					t.Errorf("output %q missing %q", s, w)
				}
			}
			if strings.Contains(s, "ghp_FAKE") {
				t.Errorf("token leaked in output: %q", s)
			}
			// second run never changes the profile again
			before := npmTokenTestRead(t, zshrc)
			out.Reset()
			if err := ensureNPMToken(&out, dirs, stubGH{auth: []byte(c.auth)}, c.interactive, refresh); err != nil {
				t.Fatalf("second run err: %v", err)
			}
			if got := npmTokenTestRead(t, zshrc); got != before {
				t.Errorf("second run changed profile: %q -> %q", before, got)
			}
			if strings.Contains(out.String(), "ghp_FAKE") {
				t.Errorf("token leaked in second-run output: %q", out.String())
			}
		})
	}
}

func TestNPMToken_EnsureNPMToken_RefreshError(t *testing.T) {
	zshrc, dirs := npmTokenTestEnv(t, true)
	refresh := func() error { return errors.New("browser flow aborted") }
	var out bytes.Buffer
	err := ensureNPMToken(&out, dirs, stubGH{auth: []byte(npmTokenTestAuthNoScope)}, true, refresh)
	if err == nil || !strings.Contains(err.Error(), "browser flow aborted") || !strings.Contains(err.Error(), "gh auth refresh") {
		t.Fatalf("want refresh error surfaced, got %v", err)
	}
	if got := npmTokenTestRead(t, zshrc); got != "" {
		t.Errorf("profile touched after refresh error: %q", got)
	}
}

func npmTokenCheckRun(t *testing.T, ghpkg bool, auth, profile, envToken string) (bool, string) {
	t.Helper()
	_, dirs := npmTokenTestEnv(t, ghpkg)
	t.Setenv("NPM_TOKEN", envToken)
	c := npmTokenCheck(func() []string { return dirs }, stubGH{auth: []byte(auth)}, func() string { return profile })
	if c.Name != "npm token (GitHub Packages)" {
		t.Errorf("name = %q", c.Name)
	}
	ok, detail := c.Run()
	if strings.Contains(detail, "ghp_FAKE") || (envToken != "" && strings.Contains(detail, envToken)) {
		t.Errorf("token leaked in detail: %q", detail)
	}
	return ok, detail
}

func TestNPMTokenCheck(t *testing.T) {
	loggedOut := "You are not logged into any GitHub hosts. To log in, run: gh auth login\n"
	cases := []struct {
		name    string
		ghpkg   bool
		auth    string
		profile string
		env     string
		wantOK  bool
		want    string
	}{
		{"not needed", false, loggedOut, "", "", true, "n/a — no repo uses GitHub Packages"},
		{"scope missing", true, npmTokenTestAuthNoScope, npmTokenTestWantLine, "", false, "gh lacks read:packages — run `gh auth refresh -h github.com -s read:packages`"},
		{"logged out", true, loggedOut, npmTokenTestWantLine, "", false, "gh not logged in — run `gh auth login`"},
		{"profile lacks marker, env empty", true, npmTokenTestAuthWithScope, "alias ll='ls -l'\n", "", false, "NPM_TOKEN not set — run `zenify up`"},
		{"profile lacks marker, env set by user", true, npmTokenTestAuthWithScope, "", "user-set-value", true, "ok"},
		{"marker in profile", true, npmTokenTestAuthWithScope, npmTokenTestWantLine + "\n", "", true, "ok"},
		{"user token in profile", true, npmTokenTestAuthWithScope, "export NPM_TOKEN=abc\n", "", true, "ok"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, detail := npmTokenCheckRun(t, c.ghpkg, c.auth, c.profile, c.env)
			if ok != c.wantOK || detail != c.want {
				t.Errorf("got (%v, %q), want (%v, %q)", ok, detail, c.wantOK, c.want)
			}
		})
	}
}

// Production source must stay project-agnostic.
func TestNPMToken_ProductionSourceNeutral(t *testing.T) {
	b, err := os.ReadFile("npmtoken.go")
	if err != nil {
		t.Fatalf("read npmtoken.go: %v", err)
	}
	s := string(b)
	for _, forbidden := range []string{"zenifyaicontactcenter", "contact-center"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("npmtoken.go must NOT contain %q", forbidden)
		}
	}
}

func strPtr(s string) *string { return &s }
