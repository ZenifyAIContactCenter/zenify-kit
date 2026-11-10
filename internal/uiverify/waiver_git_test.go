package uiverify

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// execGit is a real Deps.RunGit for these tests, with the same "--stdin
// <content>" convention as the production RunGit in internal/cli.
func execGit(dir string, args ...string) (string, error) {
	var stdin *strings.Reader
	for i, a := range args {
		if a == "--stdin" && i+1 < len(args) {
			stdin = strings.NewReader(args[i+1])
			args = args[:i+1]
			break
		}
	}
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec // G204 -- test fixture, fixed "git"
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%w: %s", err, errOut.String())
	}
	return out.String(), nil
}

const oldWithMarker = "export const a = 1\nexport const b = 2\nexport const c = 3\n" +
	"// znf:ui-verify-ok: old reason from last quarter\nexport const d = 4\n"

// waiverRepo commits Old.tsx (already carrying a marker) and App.tsx on main,
// then checks out branch feat.
func waiverRepo(t *testing.T) (string, func(args ...string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if out, err := execGit(repo, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	writeRepoFile(t, repo, "src/components/Old.tsx", oldWithMarker)
	writeRepoFile(t, repo, "src/App.tsx", "export const app = 1\n")
	git("add", "-A")
	git("commit", "-m", "base")
	git("branch", "-M", "main")
	git("checkout", "-b", "feat")
	return repo, git
}

func writeRepoFile(t *testing.T, repo, rel, body string) {
	t.Helper()
	p := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func realWaiver(t *testing.T, repo string) (string, bool) {
	t.Helper()
	d := Deps{RunGit: execGit}
	files, err := RenderSet(d, repo, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("render set is empty; the scenario changed no render file")
	}
	return Waiver(d, repo, "main", files)
}

func TestWaiver_RealGit(t *testing.T) {
	cases := []struct {
		name       string
		act        func(t *testing.T, repo string, git func(...string))
		wantOK     bool
		wantReason string
	}{
		{"committed rename keeps old marker out", func(_ *testing.T, _ string, git func(...string)) {
			git("mv", "src/components/Old.tsx", "src/components/New.tsx")
			git("commit", "-m", "rename")
		}, false, ""},
		{"staged rename keeps old marker out", func(_ *testing.T, _ string, git func(...string)) {
			git("mv", "src/components/Old.tsx", "src/components/New.tsx")
		}, false, ""},
		{"marker deleted in working tree", func(t *testing.T, repo string, git func(...string)) {
			writeRepoFile(t, repo, "src/App.tsx", "export const app = 1\n// znf:ui-verify-ok: copy only\n")
			git("commit", "-am", "waive")
			writeRepoFile(t, repo, "src/App.tsx", "export const app = 2\n")
		}, false, ""},
		{"marker added and committed", func(t *testing.T, repo string, git func(...string)) {
			writeRepoFile(t, repo, "src/App.tsx", "export const app = 1\n// znf:ui-verify-ok: copy only\n")
			git("commit", "-am", "waive")
		}, true, "copy only"},
		{"user diff config cannot hide an added marker", func(t *testing.T, repo string, git func(...string)) {
			git("config", "color.ui", "always")
			git("config", "diff.noprefix", "true")
			git("config", "diff.mnemonicPrefix", "true")
			writeRepoFile(t, repo, "src/App.tsx", "export const app = 1\n// znf:ui-verify-ok: copy only\n")
			git("commit", "-am", "waive")
		}, true, "copy only"},
		{"textconv cannot fold an old marker into an added line", func(t *testing.T, repo string, git func(...string)) {
			git("config", "diff.flatten.textconv", `tr '\n' ' ' <`)
			writeRepoFile(t, repo, ".gitattributes", "*.tsx diff=flatten\n")
			writeRepoFile(t, repo, "src/components/Old.tsx", strings.Replace(oldWithMarker, "d = 4", "d = 5", 1))
		}, false, ""},
		{"marker added in working tree", func(t *testing.T, repo string, _ func(...string)) {
			writeRepoFile(t, repo, "src/App.tsx", "export const app = 1\n// znf:ui-verify-ok: wip\n")
		}, true, "wip"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, git := waiverRepo(t)
			tc.act(t, repo, git)
			reason, ok := realWaiver(t, repo)
			if ok != tc.wantOK || reason != tc.wantReason {
				t.Errorf("Waiver = (%q, %v), want (%q, %v)", reason, ok, tc.wantReason, tc.wantOK)
			}
		})
	}
}

func TestCheck_RealGitRenameIsRequired(t *testing.T) {
	repo, git := waiverRepo(t)
	git("mv", "src/components/Old.tsx", "src/components/New.tsx")
	git("commit", "-m", "rename")
	state, msg, err := Check(Deps{RunGit: execGit}, repo, "main")
	if state != "required" || err == nil {
		t.Fatalf("state=%q msg=%q err=%v, want required", state, msg, err)
	}
}
