package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func TestUIVerifyCmd_Registered(t *testing.T) {
	root := NewRootCmd()
	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "ui-verify" {
			found = true
		}
	}
	if !found {
		t.Fatal("`ui-verify` command not registered on root")
	}
}

// initUIVerifyRepo creates a TempDir git repo with one commit (a
// non-rendering file), so tests exercise the real production RunGit against
// a real git binary.
func initUIVerifyRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...) //nolint:gosec // G204 -- test fixture, fixed argv
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out.String())
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "main.go")
	run("commit", "-m", "init")
	return repo
}

func runUIVerifyCheck(repo, base string) (string, error) {
	c := newUIVerifyCheckCmd()
	buf := &bytes.Buffer{}
	c.SetOut(buf)
	c.SetErr(buf)
	c.SetArgs([]string{"--repo", repo, "--base", base})
	err := c.Execute()
	return buf.String(), err
}

func TestUIVerifyCheck_NoRenderChanges_ExitZero(t *testing.T) {
	repo := initUIVerifyRepo(t)
	out, err := runUIVerifyCheck(repo, "HEAD")
	if err != nil {
		t.Fatalf("expected exit 0, got err=%v (out=%s)", err, out)
	}
	if !strings.Contains(out, "not_required") {
		t.Errorf("expected not_required state in output, got %q", out)
	}
}

func TestUIVerifyCheck_RenderChangedNoArtifact_Fail(t *testing.T) {
	repo := initUIVerifyRepo(t)
	compDir := filepath.Join(repo, "src", "components")
	if err := os.MkdirAll(compDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "Foo.tsx"), []byte("export {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runUIVerifyCheck(repo, "HEAD")
	if err == nil {
		t.Fatal("expected an error when a rendering file changed with no recorded artifact")
	}
	if exitcode.Code(err) != exitcode.Fail {
		t.Errorf("expected exitcode.Fail, got %d (%v)", exitcode.Code(err), err)
	}
}

func TestUIVerifyCheck_MissingBase_BadArgs(t *testing.T) {
	repo := initUIVerifyRepo(t)
	c := newUIVerifyCheckCmd()
	buf := &bytes.Buffer{}
	c.SetOut(buf)
	c.SetErr(buf)
	c.SetArgs([]string{"--repo", repo})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected an error when --base is missing")
	}
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("expected exitcode.BadArgs, got %d (%v)", exitcode.Code(err), err)
	}
}

// TestRealRunGit_StdinPiping is the load-bearing contract: the trailing arg
// after "--stdin" must be piped to the git process's stdin (not passed as an
// argv pathspec), and the returned hash must be untrimmed raw stdout apart
// from what git itself outputs — proven here by cross-checking against a
// direct `git hash-object --stdin` invocation with the same content on
// stdin.
func TestRealRunGit_StdinPiping(t *testing.T) {
	repo := initUIVerifyRepo(t)
	content := "hello uiverify\nsecond line\n"

	got, err := realRunGit(repo, "hash-object", "--stdin", content)
	if err != nil {
		t.Fatalf("realRunGit hash-object --stdin: %v", err)
	}

	cmd := exec.Command("git", "-C", repo, "hash-object", "--stdin") //nolint:gosec // G204 -- test fixture
	cmd.Stdin = strings.NewReader(content)
	want, err := cmd.Output()
	if err != nil {
		t.Fatalf("reference git hash-object --stdin: %v", err)
	}
	if got != string(want) {
		t.Errorf("realRunGit hash-object output = %q, want %q (content must be piped to stdin, not passed as argv)", got, string(want))
	}
}

// TestRealRunGit_RawStdoutNoTrim proves rev-parse output keeps its trailing
// newline — Fingerprint relies on concatenating raw RunGit outputs.
func TestRealRunGit_RawStdoutNoTrim(t *testing.T) {
	repo := initUIVerifyRepo(t)
	out, err := realRunGit(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("realRunGit rev-parse HEAD: %v", err)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("expected raw stdout with trailing newline, got %q", out)
	}
}

// writeRenderChange leaves an uncommitted rendering file in repo, so Check
// returns state "required" (no artifact for the fingerprint).
func writeRenderChange(t *testing.T, repo string) {
	t.Helper()
	compDir := filepath.Join(repo, "src", "components")
	if err := os.MkdirAll(compDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "Foo.tsx"), []byte("export {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// FR-08.4: under autonomous mode a diff that needs ui-verify stops at a
// needs-human await instead of a BLOCKED ship with no way out.
func TestUIVerifyCheckAutonomousAwaits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZENIFY_HOME", home)
	t.Setenv("ZNF_AUTONOMOUS", "1")
	repo := initUIVerifyRepo(t)
	writeRenderChange(t, repo)

	out, err := runUIVerifyCheck(repo, "HEAD")
	if exitcode.Code(err) != exitcode.Awaiting {
		t.Fatalf("want exit %d (Awaiting), got %d (%v)", exitcode.Awaiting, exitcode.Code(err), err)
	}
	matches, _ := filepath.Glob(filepath.Join(home, "await", "needs-human-*.json"))
	if len(matches) != 1 {
		t.Fatalf("want exactly one needs-human await record, got %v", matches)
	}
	b, rerr := os.ReadFile(matches[0]) //nolint:gosec // G304 -- test temp dir
	if rerr != nil {
		t.Fatal(rerr)
	}
	var rec struct {
		ID      string `json:"id"`
		Kind    string `json:"kind"`
		Summary string `json:"summary"`
	}
	if jerr := json.Unmarshal(b, &rec); jerr != nil {
		t.Fatal(jerr)
	}
	if rec.Kind != "needs-human" || !strings.Contains(rec.Summary, "cần kiểm UI") { //znf:allow-lang
		t.Errorf("record kind=%q summary=%q", rec.Kind, rec.Summary)
	}
	if !strings.Contains(out, rec.ID) {
		t.Errorf("output %q does not print await id %q", out, rec.ID)
	}
}

func TestUIVerifyCheckInteractiveUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZENIFY_HOME", home)
	t.Setenv("ZNF_AUTONOMOUS", "")
	repo := initUIVerifyRepo(t)
	writeRenderChange(t, repo)

	_, err := runUIVerifyCheck(repo, "HEAD")
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("want exit %d (Fail), got %d (%v)", exitcode.Fail, exitcode.Code(err), err)
	}
	if _, serr := os.Stat(filepath.Join(home, "await")); !os.IsNotExist(serr) {
		t.Errorf("await dir created outside autonomous mode (stat err=%v)", serr)
	}
}

// `ui-verify files` prints the rendering files one per line, the list ship counts to decide on a verifier.
func TestUIVerifyFiles_ListsRenderingFilesOnly(t *testing.T) {
	repo := initUIVerifyRepo(t)
	compDir := filepath.Join(repo, "src", "components")
	if err := os.MkdirAll(compDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"Foo.tsx": "export {}\n", "Foo.test.tsx": "test()\n"} {
		if err := os.WriteFile(filepath.Join(compDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	c := newUIVerifyCmd()
	c.SetOut(&buf)
	c.SetArgs([]string{"files", "--repo", repo, "--base", "HEAD"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "src/components/Foo.tsx\n" {
		t.Fatalf("files output = %q", got)
	}
}

func gitIn(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...) //nolint:gosec // G204 -- test fixture, fixed argv
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func runUIVerifyFiles(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	c := newUIVerifyCmd()
	c.SetOut(&buf)
	c.SetArgs(append([]string{"files"}, args...))
	err := c.Execute()
	return buf.String(), err
}

// A committed test file with a non-ASCII name is still a test file: git must not quote it.
func TestUIVerifyFiles_NonASCIITestFileSkipped(t *testing.T) {
	repo := initUIVerifyRepo(t)
	base := strings.TrimSpace(uiGitOut(t, repo, "rev-parse", "HEAD"))
	dir := filepath.Join(repo, "src", "components")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Gr\u00f6\u00dfe.test.tsx"), []byte("test()\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-m", "test only")
	out, err := runUIVerifyFiles(t, "--repo", repo, "--base", base, "--count")
	if err != nil || out != "0\n" {
		t.Fatalf("count = %q, err = %v", out, err)
	}
}

// UI commits that land on the base after the branch point are not this branch's change.
func TestUIVerifyFiles_CountsFromMergeBase(t *testing.T) {
	repo := initUIVerifyRepo(t)
	gitIn(t, repo, "branch", "-M", "main")
	gitIn(t, repo, "checkout", "-b", "feat")
	if err := os.WriteFile(filepath.Join(repo, "api.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-m", "backend")
	gitIn(t, repo, "checkout", "main")
	if err := os.WriteFile(filepath.Join(repo, "App.tsx"), []byte("export {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-m", "ui on main")
	gitIn(t, repo, "checkout", "feat")
	out, err := runUIVerifyFiles(t, "--repo", repo, "--base", "main", "--count")
	if err != nil || out != "0\n" {
		t.Fatalf("count = %q, err = %v", out, err)
	}
}

// --count fails closed: a bad base is an error, never a printed 0.
func TestUIVerifyFiles_CountErrorPrintsNothing(t *testing.T) {
	repo := initUIVerifyRepo(t)
	out, err := runUIVerifyFiles(t, "--repo", repo, "--base", "no-such-ref", "--count")
	if err == nil || out != "" {
		t.Fatalf("out = %q, err = %v; want an error and no output", out, err)
	}
}

func uiGitOut(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).Output() //nolint:gosec // G204 -- test fixture, fixed argv
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func runUIVerifyBlocked(t *testing.T, args ...string) (string, error) {
	t.Helper()
	c := newUIVerifyBlockedCmd()
	buf := &bytes.Buffer{}
	c.SetOut(buf)
	c.SetErr(buf)
	c.SetArgs(args)
	err := c.Execute()
	return buf.String(), err
}

func blockedFiles(repo string) []string {
	m, _ := filepath.Glob(filepath.Join(repo, ".znf", "ui-verify", "*.blocked.json"))
	return m
}

func TestUIVerifyBlockedRefusedOutsideAutonomous(t *testing.T) {
	t.Setenv("ZNF_AUTONOMOUS", "")
	repo := initUIVerifyRepo(t)
	_, err := runUIVerifyBlocked(t, "--repo", repo, "--reason", "deps_auth_failed:x")
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("want exit %d (Fail), got %d (%v)", exitcode.Fail, exitcode.Code(err), err)
	}
	if got := blockedFiles(repo); len(got) != 0 {
		t.Errorf("blocked record written outside autonomous mode: %v", got)
	}
}

func TestUIVerifyBlockedEmptyReasonRefused(t *testing.T) {
	t.Setenv("ZNF_AUTONOMOUS", "1")
	repo := initUIVerifyRepo(t)
	_, err := runUIVerifyBlocked(t, "--repo", repo, "--reason", "")
	if err == nil || exitcode.Code(err) == 0 {
		t.Fatalf("want non-zero exit for empty reason, got %v", err)
	}
	if got := blockedFiles(repo); len(got) != 0 {
		t.Errorf("blocked record written for empty reason: %v", got)
	}
}

// The reason is a machine code the zenie host matches with
// ^[a-z_]{1,40}(?::[A-Za-z0-9._-]{1,60})?$; anything else must be refused here,
// not silently dropped there.
func TestUIVerifyBlockedReasonShape(t *testing.T) {
	t.Setenv("ZNF_AUTONOMOUS", "1")
	for _, tc := range []struct {
		reason string
		ok     bool
	}{
		{"app_wont_start", true},
		{"deps_auth_failed:contact-center-hub", true},
		{"drive_failed:global-search", true},
		{"drive_failed:deals-id", true},
		{"  app_wont_start  ", true},
		{"drive_failed:/deals/:id", false},
		{"   ", false},
		{"deps_auth_failed\nstate=verified", false},
		{"Drive_failed:x", false},
		{"drive failed", false},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			repo := initUIVerifyRepo(t)
			reason := tc.reason
			_, err := runUIVerifyBlocked(t, "--repo", repo, "--reason", reason)
			if tc.ok {
				if err != nil {
					t.Fatalf("valid reason %q refused: %v", reason, err)
				}
				return
			}
			if exitcode.Code(err) != exitcode.BadArgs {
				t.Fatalf("reason %q: err=%v code=%d, want BadArgs", reason, err, exitcode.Code(err))
			}
			if got := blockedFiles(repo); len(got) != 0 {
				t.Errorf("blocked record written for %q: %v", reason, got)
			}
		})
	}
}

func TestUIVerifyBlockedWritesStatusFile(t *testing.T) {
	t.Setenv("ZNF_AUTONOMOUS", "1")
	status := filepath.Join(t.TempDir(), "s.json")
	t.Setenv("ZNF_UI_STATUS_FILE", status)
	repo := initUIVerifyRepo(t)
	out, err := runUIVerifyBlocked(t, "--repo", repo, "--reason", "app_wont_start")
	if err != nil {
		t.Fatalf("err=%v out=%s", err, out)
	}
	b, rerr := os.ReadFile(status) //nolint:gosec // G304 -- test temp dir
	if rerr != nil {
		t.Fatal(rerr)
	}
	var rec struct{ State, Reason, Repo, FP string }
	if err := json.Unmarshal(b, &rec); err != nil || rec.State != "infra-blocked" || rec.Reason != "app_wont_start" || rec.Repo != filepath.Clean(repo) || rec.FP == "" {
		t.Errorf("status file = %q (%v)", b, err)
	}
	if got := blockedFiles(repo); len(got) != 1 {
		t.Errorf("want one blocked record, got %v", got)
	}
}

func TestUIVerifyBlockedStatusFileErrorStillExitsZero(t *testing.T) {
	t.Setenv("ZNF_AUTONOMOUS", "1")
	t.Setenv("ZNF_UI_STATUS_FILE", filepath.Join(t.TempDir(), "missing-dir", "s.json"))
	repo := initUIVerifyRepo(t)
	out, err := runUIVerifyBlocked(t, "--repo", repo, "--reason", "app_wont_start")
	if err != nil {
		t.Fatalf("want exit 0, got %v (%s)", err, out)
	}
	if !strings.Contains(out, "warning") {
		t.Errorf("expected a warning in output, got %q", out)
	}
}

func TestUIVerifyCheckInfraBlockedAutonomousNoAwait(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZENIFY_HOME", home)
	t.Setenv("ZNF_AUTONOMOUS", "1")
	t.Setenv("ZNF_UI_STATUS_FILE", "")
	repo := initUIVerifyRepo(t)
	writeRenderChange(t, repo)
	if _, err := runUIVerifyBlocked(t, "--repo", repo, "--reason", "deps_auth_failed:hub"); err != nil {
		t.Fatal(err)
	}
	out, err := runUIVerifyCheck(repo, "HEAD")
	if err != nil {
		t.Fatalf("want exit 0, got %v (%s)", err, out)
	}
	if !strings.Contains(out, "infra-blocked: deps_auth_failed:hub") {
		t.Errorf("output %q lacks the infra-blocked state", out)
	}
	if _, serr := os.Stat(filepath.Join(home, "await")); !os.IsNotExist(serr) {
		t.Errorf("await dir created for infra-blocked (stat err=%v)", serr)
	}
}

func TestUIVerifyCheckInfraBlockedInteractiveFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZENIFY_HOME", home)
	t.Setenv("ZNF_AUTONOMOUS", "1")
	t.Setenv("ZNF_UI_STATUS_FILE", "")
	repo := initUIVerifyRepo(t)
	writeRenderChange(t, repo)
	if _, err := runUIVerifyBlocked(t, "--repo", repo, "--reason", "deps_auth_failed:hub"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZNF_AUTONOMOUS", "")
	_, err := runUIVerifyCheck(repo, "HEAD")
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("want exit %d (Fail), got %d (%v)", exitcode.Fail, exitcode.Code(err), err)
	}
	if _, serr := os.Stat(filepath.Join(home, "await")); !os.IsNotExist(serr) {
		t.Errorf("await dir created outside autonomous mode (stat err=%v)", serr)
	}
}

// ZNF_UI_STATUS_FILE reports infra-blocked to the host. It is one file per run,
// so only a check of the repo that wrote it may clear it: a check of another repo
// in the same ship (not_required) leaves it, a later verified check of the same
// repo (newer fp) removes it, and a missing file is no error.
func TestUIVerifyCheckClearsStaleStatusFile(t *testing.T) {
	t.Setenv("ZENIFY_HOME", t.TempDir())
	t.Setenv("ZNF_AUTONOMOUS", "1")
	status := filepath.Join(t.TempDir(), "s.json")
	t.Setenv("ZNF_UI_STATUS_FILE", status)

	web := initUIVerifyRepo(t)
	writeRenderChange(t, web)
	if _, err := runUIVerifyBlocked(t, "--repo", web, "--reason", "app_wont_start"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(status) //nolint:gosec // G304 -- test temp dir
	if err != nil {
		t.Fatal(err)
	}
	var rec struct{ State, Reason, Repo, FP string }
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatalf("status file %q: %v", b, err)
	}
	if rec.State != "infra-blocked" || rec.Reason != "app_wont_start" || rec.Repo != filepath.Clean(web) || rec.FP == "" {
		t.Fatalf("status file = %+v, want state/reason/repo=%s/fp", rec, web)
	}

	// infra-blocked check of the same repo keeps it.
	if out, err := runUIVerifyCheck(web, "HEAD"); err != nil {
		t.Fatalf("infra-blocked check: %v (%s)", err, out)
	}
	if _, err := os.Stat(status); err != nil {
		t.Fatalf("infra-blocked check removed the status file: %v", err)
	}

	// another repo of the same ship (not_required) leaves it.
	be := initUIVerifyRepo(t)
	if out, err := runUIVerifyCheck(be, "HEAD"); err != nil {
		t.Fatalf("other-repo check: %v (%s)", err, out)
	}
	if _, err := os.Stat(status); err != nil {
		t.Fatalf("a check of another repo removed the status file: %v", err)
	}

	// the same repo later verifies (new fp): the file goes.
	if err := os.WriteFile(filepath.Join(web, "src", "components", "Foo.tsx"), []byte("export const a = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shot := filepath.Join(t.TempDir(), "s.png")
	if err := os.WriteFile(shot, []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	rc := newUIVerifyRecordCmd()
	rc.SetOut(&bytes.Buffer{})
	rc.SetArgs([]string{"--repo", web, "--screen", "home", "--screenshot", shot, "--child-right", "1", "--container-right", "2", "--padding-right", "0"})
	if err := rc.Execute(); err != nil {
		t.Fatal(err)
	}
	out, err := runUIVerifyCheck(web, "HEAD")
	if err != nil || !strings.Contains(out, "verified") {
		t.Fatalf("verified check: %v (%s)", err, out)
	}
	if _, err := os.Stat(status); !os.IsNotExist(err) {
		t.Fatalf("status file survived a verified check of its repo: %v", err)
	}

	// already gone: still exit 0.
	if out, err := runUIVerifyCheck(web, "HEAD"); err != nil {
		t.Fatalf("check with no status file: %v (%s)", err, out)
	}

	// unparseable file: left alone.
	if err := os.WriteFile(status, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runUIVerifyCheck(web, "HEAD"); err != nil {
		t.Fatalf("check with a broken status file: %v (%s)", err, out)
	}
	if _, err := os.Stat(status); err != nil {
		t.Fatalf("unparseable status file removed: %v", err)
	}
}
