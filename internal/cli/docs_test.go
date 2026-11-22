package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsview"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// docs sync resolves the repo dir via discovery (repos/<repo> or flat).
func TestDocsDirResolves(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "docs")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveWorkspaceRepoDir(ws, defaultDocsRepo, "", os.ReadDir)
	if filepath.Clean(got) != filepath.Clean(repo) {
		t.Fatalf("resolve docs dir: got %q want %q", got, repo)
	}
}

// --dir override must win: resolveDocsStore must not be consulted, and the store
// actually used is exactly --dir (not the path resolveDocsStore would return),
// shown via the link farm created under workspace/docs → --dir/specs.
func TestDocsSyncDirFlagWins(t *testing.T) {
	ws := t.TempDir()
	store := t.TempDir() // NOT a git repo, NOT under ws — entirely different from where resolveDocsStore would point
	if err := os.MkdirAll(filepath.Join(store, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := newDocsCmd()
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"sync", "--workspace", ws, "--dir", store})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// viewDir = ws/docs, must differ from store so EnsureView runs and links to store/specs.
	viewDir := filepath.Join(ws, defaultDocsRepo)
	if _, err := os.Stat(filepath.Join(viewDir, "specs")); err != nil {
		t.Fatalf("view farm not created at %s: %v (--dir may NOT have won)", viewDir, err)
	}
	same, err := (docsview.OSFS{}).SameTarget(filepath.Join(viewDir, "specs"), filepath.Join(store, "specs"))
	if err != nil || !same {
		t.Fatalf("view/specs does not point to --dir/specs: same=%v err=%v", same, err)
	}
}

// EnsureView must NOT run when viewDir == dir (not migrated: docs store still
// lives right in the workspace) — no "docs view:" note is printed.
func TestDocsSyncEnsureViewSkippedWhenViewEqualsDir(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, defaultDocsRepo) // == viewDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := newDocsCmd()
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"sync", "--workspace", ws, "--dir", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if strings.Contains(errBuf.String(), "docs view:") {
		t.Fatalf("EnsureView must be skipped when viewDir==dir, but got note: %s", errBuf.String())
	}
}

func TestStoreDirsPinned(t *testing.T) {
	want := []string{"specs", "plans", "reference", "handoff", "releases", "agent-memory"}
	if !reflect.DeepEqual(storeDirs, want) {
		t.Fatalf("storeDirs = %v, want %v", storeDirs, want)
	}
	if len(docsDirs) != 11 {
		t.Fatalf("docsDirs = %v", docsDirs)
	}
}

// SC-1 (migrated): five local dirs are real dirs, six store dirs are links; second run adds no note.
func TestDocsSyncCreatesLocalDirs(t *testing.T) {
	ws, store := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(store, "specs"), 0o755)
	run := func() string {
		cmd := newDocsCmd()
		var e bytes.Buffer
		cmd.SetErr(&e)
		cmd.SetArgs([]string{"sync", "--workspace", ws, "--dir", store})
		cmd.Execute()
		return e.String()
	}
	run()
	for _, d := range localDirs {
		fi, err := os.Lstat(filepath.Join(ws, "docs", d))
		if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s: %v %v", d, fi, err)
		}
	}
	if fi, _ := os.Lstat(filepath.Join(ws, "docs", "specs")); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("specs must be a link")
	}
	if out := run(); strings.Contains(out, "docs local:") {
		t.Fatalf("second run noted: %s", out)
	}
}

// SC-1 (not migrated): viewDir == store, five dirs created inside the store.
func TestDocsSyncLocalDirsWhenViewEqualsDir(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, "docs")
	os.MkdirAll(dir, 0o755)
	cmd := newDocsCmd()
	cmd.SetArgs([]string{"sync", "--workspace", ws, "--dir", dir})
	cmd.Execute()
	for _, d := range localDirs {
		if fi, err := os.Stat(filepath.Join(dir, d)); err != nil || !fi.IsDir() {
			t.Fatalf("%s missing", d)
		}
	}
}

func TestDocsSweep_NoSpecsDirFailsOpen(t *testing.T) { // SC-8
	home := t.TempDir()
	t.Setenv("ZENIFY_HOME", home)
	os.MkdirAll(filepath.Join(home, "knowledge"), 0o755)
	var e bytes.Buffer
	cmd := newDocsCmd()
	cmd.SetErr(&e)
	cmd.SetArgs([]string{"sweep", "--apply", "--workspace", t.TempDir()})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(e.String(), "docs sweep:"); n != 1 {
		t.Fatalf("want one note, got %q", e.String())
	}
}

func TestDocsSweep_DryRunPrintsPlanAndTouchesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZENIFY_HOME", home)
	store := filepath.Join(home, "knowledge")
	writeF(t, filepath.Join(store, "specs", "r", "2026-01-01-a-design.md"), "# a\n")
	writeF(t, filepath.Join(store, "handoff", "ROADMAP.md"), "R")
	var e bytes.Buffer
	cmd := newDocsCmd()
	cmd.SetErr(&e)
	cmd.SetArgs([]string{"sweep", "--no-fetch", "--workspace", t.TempDir()})
	cmd.Execute()
	if !strings.Contains(e.String(), "docs sweep: delete-handoff handoff/ROADMAP.md (undated)") || !strings.Contains(e.String(), "dry-run") {
		t.Fatal(e.String())
	}
	if _, err := os.Stat(filepath.Join(store, "handoff", "ROADMAP.md")); err != nil {
		t.Fatal("dry-run deleted a file")
	}
}

// --dir scopes the sweep to an explicit store: ZENIFY_HOME's store is never touched, and with no
// --dir the store still resolves through ZENIFY_HOME. HOME and ZENIFY_HOME both point at temp dirs.
func TestDocsSweep_DirScopesStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	zh := t.TempDir()
	t.Setenv("ZENIFY_HOME", zh)
	real := filepath.Join(zh, "knowledge")
	clone := filepath.Join(t.TempDir(), "clone")
	third := filepath.Join(t.TempDir(), "third")
	for _, s := range []string{real, clone, third} {
		writeF(t, filepath.Join(s, "specs", "r", "2026-01-01-a-design.md"), "# a\n")
		writeF(t, filepath.Join(s, "handoff", "old.md"), "x")
	}
	exists := func(s string) bool {
		_, err := os.Stat(filepath.Join(s, "handoff", "old.md"))
		return err == nil
	}
	if err := docsSweepCore(t.TempDir(), clone, true, true, io.Discard); err != nil {
		t.Fatal(err)
	}
	if exists(clone) {
		t.Fatal("--dir store was not swept")
	}
	if !exists(real) || !exists(third) {
		t.Fatal("sweep with an explicit dir touched another store")
	}
	var e bytes.Buffer
	if err := docsSweepCore(t.TempDir(), "", false, true, &e); err != nil {
		t.Fatal(err)
	}
	if !exists(real) || !strings.Contains(e.String(), "delete-handoff handoff/old.md") {
		t.Fatalf("default path should resolve to the ZENIFY_HOME store: %s", e.String())
	}
	if err := docsSweepCore(t.TempDir(), "", true, true, io.Discard); err != nil {
		t.Fatal(err)
	}
	if exists(real) {
		t.Fatal("default path did not resolve to the ZENIFY_HOME store")
	}
}

// sweepStore builds a bare remote and a store clone tracking it, seeded with one spec and one
// undated handoff, under temp dirs with an isolated git config.
func sweepStore(t *testing.T) (store string, run func(dir string, args ...string)) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "t@example.invalid")
	}
	run = func(dir string, args ...string) {
		if out, err := gitx.ExecRunner().Run(dir, args...); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	store = filepath.Join(root, "store")
	run(root, "init", "-q", "--bare", "-b", "main", remote)
	run(root, "clone", "-q", remote, store)
	writeF(t, filepath.Join(store, "specs", "r", "2026-01-01-a-design.md"), "# a\n")
	writeF(t, filepath.Join(store, "handoff", "old.md"), "x")
	run(store, "add", "-A")
	run(store, "commit", "-qm", "seed")
	run(store, "push", "-q", "origin", "HEAD:main")
	run(store, "branch", "-q", "--set-upstream-to=origin/main")
	return store, run
}

// --apply on a store that cannot be brought up to date with its remote (here: dirty) writes
// nothing and says so: a delete or header written onto a stale base conflicts on the next sync.
func TestDocsSweep_ApplySkippedWhenStoreNotUpToDate(t *testing.T) {
	store, _ := sweepStore(t)
	writeF(t, filepath.Join(store, "notes.md"), "uncommitted")

	var e bytes.Buffer
	if err := docsSweepCore(t.TempDir(), store, true, true, &e); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store, "handoff", "old.md")); err != nil {
		t.Fatal("apply wrote into a store that is not up to date")
	}
	if !strings.Contains(e.String(), "bỏ --apply") { //znf:allow-lang
		t.Fatalf("missing skip note: %s", e.String())
	}
}

// The incident path: a clean store behind its remote is pulled BEFORE planning, so --apply acts
// on the teammate's newer content (here: a handoff only the remote has is swept too).
func TestDocsSweep_ApplyPullsBehindStoreBeforePlanning(t *testing.T) {
	store, run := sweepStore(t)
	mate := filepath.Join(filepath.Dir(store), "mate")
	run(filepath.Dir(store), "clone", "-q", filepath.Join(filepath.Dir(store), "remote.git"), mate)
	writeF(t, filepath.Join(mate, "handoff", "remote-only.md"), "y")
	run(mate, "add", "-A")
	run(mate, "commit", "-qm", "mate")
	run(mate, "push", "-q")

	if err := docsSweepCore(t.TempDir(), store, true, true, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store, "notes", "remote-only.md")); err != nil {
		t.Fatal("store was not pulled before planning: the remote-only handoff was not swept")
	}
}
