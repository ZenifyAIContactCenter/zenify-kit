package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// nopRunner: git is never called when the workspace is empty (no participant repo).
type nopRunner struct{}

func (nopRunner) Run(dir string, args ...string) ([]byte, error) { return nil, nil }

// SC-7: production source (excluding _test.go) of internal/release + internal/cli
// must NOT hardcode a zenify repo identifier → keeps the kit project-agnostic.
func TestNoZenifyRepoIdentifiersInSource(t *testing.T) {
	// Ban identifiers of repos THAT PARTICIPATE in release (this list must live in .zenify/release-repos.txt).
	// Deliberate exception: "docs" is the record-layer repo (M6a, renamed from zenify-knowledge)
	// used as the default out-dir, and already has the --out-dir flag to override → not banned.
	banned := []string{
		"contact-center-be", "contact-center-hub", "contact-center-web",
		"chatting", "notification", "personal-zalo-gateway", "change-stream-subscriber",
	}
	dirs := []string{".", "../release"}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			t.Fatalf("read dir %s: %v", d, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(d, name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			src := string(b)
			for _, id := range banned {
				if strings.Contains(src, id) {
					t.Errorf("%s/%s contains zenify repo identifier %q — kit must stay project-agnostic (move it into .zenify/release-repos.txt)", d, name, id)
				}
			}
		}
	}
}

// FAIL-OPEN: workspace has no repo at all → still writes an empty report, exit 0 (FR-5.2).
// ZENIFY_HOME=t.TempDir() to isolate from the test machine's real ~/.zenify/knowledge.
func TestRunReleaseReportFailOpenEmptyWorkspace(t *testing.T) {
	ws := t.TempDir()
	zh := t.TempDir()
	t.Setenv("ZENIFY_HOME", zh)
	var out, errb bytes.Buffer
	if err := runReleaseReport(ws, "v261104.0", true, "", false, nopRunner{}, &out, &errb); err != nil {
		t.Fatalf("fail-open violated: returned err %v", err)
	}
	path := filepath.Join(zh, "knowledge", "releases", "v261104.0.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("an empty report must still be written: %v", err)
	}
	if !strings.Contains(string(b), "# Release v261104.0") {
		t.Errorf("report missing header: %s", b)
	}
}

// verbose=true on an empty workspace: still fail-open, report still written, no panic (FR-5.3).
func TestRunReleaseReportVerboseFailOpen(t *testing.T) {
	ws := t.TempDir()
	zh := t.TempDir()
	t.Setenv("ZENIFY_HOME", zh)
	var out, errb bytes.Buffer
	if err := runReleaseReport(ws, "v261104.0", true, "", true, nopRunner{}, &out, &errb); err != nil {
		t.Fatalf("fail-open violated (verbose): returned err %v", err)
	}
	path := filepath.Join(zh, "knowledge", "releases", "v261104.0.md")
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("verbose report must still be written: %v", err)
	}
}

// initTaggedRepo makes a real git repo at dir with one empty commit per subject, tagging after each
// commit whose tag is non-empty. Global/system git config is isolated so signing cannot interfere.
func initTaggedRepo(t *testing.T, dir string, steps [][2]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t",
			"-c", "commit.gpgsign=false", "-c", "tag.gpgSign=false"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	for _, s := range steps {
		git("commit", "-q", "--allow-empty", "-m", s[0])
		if s[1] != "" {
			git("tag", s[1])
		}
	}
}

func isolateGit(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// No tag argument: the report is built for the newest vYYMMDD.n tag, over prevTag..tag (the
// midweek hotfix tag is the lower bound), written as <tag>.md; a pinned repo with no tag gets a
// "chưa có tag" line instead of the command exiting silently. //znf:allow-lang
func TestRunReleaseReportByTag(t *testing.T) { //znf:allow-lang
	isolateGit(t)
	ws := t.TempDir()
	zh := t.TempDir()
	t.Setenv("ZENIFY_HOME", zh)
	initTaggedRepo(t, filepath.Join(ws, "be"), [][2]string{
		{"feat(a): weekly one", "v261028.0"},
		{"fix(b): hotfix midweek", "v261028.2"},
		{"feat(c): next week", "v261104.0"},
	})
	initTaggedRepo(t, filepath.Join(ws, "bare"), [][2]string{{"chore: init", ""}})
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "release-repos.txt"), []byte("be\nbare\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if err := runReleaseReport(ws, "", true, "", true, gitx.ExecRunner(), &out, &errb); err != nil {
		t.Fatalf("fail-open violated: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(zh, "knowledge", "releases", "v261104.0.md"))
	if err != nil {
		t.Fatalf("report v261104.0.md not written: %v (stderr %s)", err, errb.String())
	}
	rep := string(b)
	for _, want := range []string{"# Release v261104.0", "## be (v261028.2..v261104.0)", "feat(c): next week", "chưa có tag vYYMMDD.n"} { //znf:allow-lang
		if !strings.Contains(rep, want) {
			t.Errorf("report missing %q:\n%s", want, rep)
		}
	}
	for _, notWant := range []string{"weekly one", "hotfix midweek"} {
		if strings.Contains(rep, notWant) {
			t.Errorf("commit %q is before v261028.2 and must not be in the report:\n%s", notWant, rep)
		}
	}
}

// No repo carries a release tag: no report file, but a stderr line says so, exit 0.
func TestRunReleaseReportNoTagAnywhere(t *testing.T) {
	isolateGit(t)
	ws := t.TempDir()
	zh := t.TempDir()
	t.Setenv("ZENIFY_HOME", zh)
	initTaggedRepo(t, filepath.Join(ws, "bare"), [][2]string{{"chore: init", ""}})
	var out, errb bytes.Buffer
	if err := runReleaseReport(ws, "", true, "", false, gitx.ExecRunner(), &out, &errb); err != nil {
		t.Fatalf("fail-open violated: %v", err)
	}
	if !strings.Contains(errb.String(), "chưa có tag") { //znf:allow-lang
		t.Errorf("stderr must say no tag: %q", errb.String())
	}
}
