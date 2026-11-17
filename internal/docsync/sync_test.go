package docsync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

type fakeRunner struct {
	calls  [][]string
	status string // value returned for `status --porcelain`
	ahead  string // value returned for `rev-list --count @{upstream}..HEAD` (empty = undetermined → 0)
	failOn string // arg[0] that will return an error (empty = no error)
}

func (f *fakeRunner) Run(dir string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if len(args) > 0 && args[0] == f.failOn {
		return nil, &runErr{args[0]}
	}
	if len(args) >= 2 && args[0] == "status" {
		return []byte(f.status), nil
	}
	if len(args) > 0 && args[0] == "rev-list" {
		return []byte(f.ahead), nil
	}
	return nil, nil
}

type runErr struct{ v string }

func (e *runErr) Error() string { return "fail:" + e.v }

func ran(calls [][]string, sub string) bool {
	for _, c := range calls {
		joined := strings.Join(c, " ")
		if joined == sub || strings.HasPrefix(joined, sub) {
			return true
		}
	}
	return false
}

// Clean + ahead=0 → only status + rev-list (both LOCAL), NO network mutation:
// no pull, no commit, no push. Guarantees the Stop hook running every turn
// (clean, up-to-date) costs no network round-trip.
func TestSync_CleanIsLocalOnly(t *testing.T) {
	f := &fakeRunner{status: "", ahead: "0"}
	Sync(f, "/docs")
	if ran(f.calls, "commit") || ran(f.calls, "pull") || ran(f.calls, "push") {
		t.Fatalf("a clean+up-to-date turn must not pull/commit/push; calls=%v", f.calls)
	}
	if !ran(f.calls, "status --porcelain") {
		t.Fatalf("must check status; calls=%v", f.calls)
	}
}

// Clean but STILL has an unpushed commit (clean-but-ahead) → must NOT commit
// extra, but must pull --rebase + push to push through the commit stuck from before.
func TestSync_CleanButAheadPushesPending(t *testing.T) {
	f := &fakeRunner{status: "", ahead: "2"}
	Sync(f, "/docs")
	if ran(f.calls, "add -A") || ran(f.calls, "commit") {
		t.Fatalf("when clean must NOT add/commit extra; calls=%v", f.calls)
	}
	if !ran(f.calls, "pull --rebase") || !ran(f.calls, "push") {
		t.Fatalf("clean-but-ahead must pull--rebase+push the stuck commit; calls=%v", f.calls)
	}
}

// ahead undetermined (upstream not set → rev-list errors) → fail-safe to clean,
// NO network touch (preserve old behavior when ahead is unknown).
func TestSync_CleanAheadUnknownStaysLocal(t *testing.T) {
	f := &fakeRunner{status: "", failOn: "rev-list"}
	Sync(f, "/docs")
	if ran(f.calls, "pull") || ran(f.calls, "push") {
		t.Fatalf("undetermined ahead must stay local, no network; calls=%v", f.calls)
	}
}

// Dirty → commit FIRST, then pull --rebase, then push (commit-first order).
func TestSync_DirtyCommitsThenRebasesThenPushes(t *testing.T) {
	f := &fakeRunner{status: " M specs/x.md"}
	Sync(f, "/docs")
	if !ran(f.calls, "add -A") || !ran(f.calls, "commit") ||
		!ran(f.calls, "pull --rebase") || !ran(f.calls, "push") {
		t.Fatalf("dirty must add+commit+pull--rebase+push; calls=%v", f.calls)
	}
	// commit must come BEFORE pull (commit-first, so a rebase conflict can be aborted).
	var ci, pi = -1, -1
	for i, c := range f.calls {
		j := strings.Join(c, " ")
		if strings.HasPrefix(j, "commit") {
			ci = i
		}
		if strings.HasPrefix(j, "pull") && pi == -1 {
			pi = i
		}
	}
	if ci == -1 || pi == -1 || ci > pi {
		t.Fatalf("commit must be before pull; commit@%d pull@%d calls=%v", ci, pi, f.calls)
	}
}

// Rebase conflict (pull errors) → rebase --abort, NO push (no publishing the marker).
func TestSync_RebaseConflictAbortsNoPush(t *testing.T) {
	f := &fakeRunner{status: " M specs/x.md", failOn: "pull"}
	Sync(f, "/docs")
	if !ran(f.calls, "rebase --abort") {
		t.Fatalf("pull error must rebase --abort; calls=%v", f.calls)
	}
	if ran(f.calls, "push") {
		t.Fatalf("pull error must NOT push (avoid publishing garbage); calls=%v", f.calls)
	}
}

// Fail-open: status error still returns notes, no panic, no network.
func TestSync_StatusFailOpen(t *testing.T) {
	f := &fakeRunner{failOn: "status"}
	notes := Sync(f, "/docs")
	if len(notes) == 0 {
		t.Fatal("status error must produce a note")
	}
	if ran(f.calls, "commit") || ran(f.calls, "push") {
		t.Fatalf("status error must not commit/push; calls=%v", f.calls)
	}
}

// A git failure must surface git's reason, not just "exit status N" — that
// opaque form hid "Author identity unknown" from users. The stderr is made by
// sh, not git, so the test does not depend on git's locale or version.
func TestGitErr_KeepsFirstAndFatalLine(t *testing.T) {
	_, err := exec.Command("sh", "-c", `printf 'Author identity unknown\n\n*** Please tell me who you are.\nRun\n  git config ...\nfatal: unable to auto-detect email address\n' >&2; exit 128`).Output()
	got := gitErr(err)
	want := "exit status 128: Author identity unknown … fatal: unable to auto-detect email address"
	if got != want {
		t.Fatalf("gitErr:\n got %q\nwant %q", got, want)
	}
}

// The note goes into the session every turn, so a huge stderr is capped.
func TestGitErr_CapsLongReason(t *testing.T) {
	_, err := exec.Command("sh", "-c", `printf '%0500d\n' 0 >&2; exit 1`).Output()
	got := gitErr(err)
	if n := len([]rune(got)); n > len("exit status 1: ")+maxReason+1 {
		t.Fatalf("reason not capped: %d runes", n)
	}
}

// gitEnv isolates real-git tests from the host config and gives them an identity.
func gitEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "t")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "t@example.invalid")
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitx.ExecRunner().Run(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %s", args, gitErr(err))
	}
	return string(out)
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const specPath = "specs/r/2026-01-01-a-design.md"

// stores builds a bare remote holding one spec, plus two clones: ours (the store) and a teammate's.
func stores(t *testing.T) (ours, mate string) {
	t.Helper()
	gitEnv(t)
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	git(t, root, "init", "-q", "--bare", "-b", "main", remote)
	seed := filepath.Join(root, "seed")
	git(t, root, "clone", "-q", remote, seed)
	writeFile(t, filepath.Join(seed, specPath), "# a\n\n**Trạng thái:** draft\n\nbody\n") //znf:allow-lang
	git(t, seed, "add", "-A")
	git(t, seed, "commit", "-qm", "seed")
	git(t, seed, "push", "-q", "origin", "HEAD:main")
	ours, mate = filepath.Join(root, "ours"), filepath.Join(root, "mate")
	git(t, root, "clone", "-q", remote, ours)
	git(t, root, "clone", "-q", remote, mate)
	return ours, mate
}

// edit rewrites the spec in dir and commits it.
func edit(t *testing.T, dir, body, msg string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, specPath), body)
	git(t, dir, "commit", "-qam", msg)
}

// The incident: our sweep rewrote a spec's status header on a stale store while a teammate changed
// the same line on the remote. Sync must drop our header-only commit, take the teammate's line and
// push the real commit stacked behind it — instead of failing the same rebase on every turn.
func TestSync_HeaderOnlyConflictIsDroppedThenPushed(t *testing.T) {
	ours, mate := stores(t)
	edit(t, mate, "# a\n\n**Trạng thái:** superseded\n\nbody\n", "mate") //znf:allow-lang
	git(t, mate, "push", "-q")
	edit(t, ours, "# a\n\n**Trạng thái:** built\n\nbody\n", "sweep header") //znf:allow-lang
	writeFile(t, filepath.Join(ours, "specs/r/2026-01-02-b-design.md"), "# b\n")
	git(t, ours, "add", "-A")
	git(t, ours, "commit", "-qm", "real")

	notes := Sync(gitx.ExecRunner(), ours)
	if len(notes) != 1 || !strings.HasPrefix(notes[0], "docs sync: pushed; dropped 1 conflicting status-header-only commit") {
		t.Fatalf("notes = %v", notes)
	}
	git(t, mate, "pull", "-q")
	b, _ := os.ReadFile(filepath.Join(mate, specPath))
	if !strings.Contains(string(b), "superseded") {
		t.Fatalf("teammate's header lost: %q", b)
	}
	if _, err := os.Stat(filepath.Join(mate, "specs/r/2026-01-02-b-design.md")); err != nil {
		t.Fatal("real commit behind the header commit was not pushed")
	}
}

// A conflict on real content still aborts and keeps our commit intact, unpushed, as before.
func TestSync_RealContentConflictStillAborts(t *testing.T) {
	ours, mate := stores(t)
	edit(t, mate, "# a\n\n**Trạng thái:** draft\n\nmate body\n", "mate") //znf:allow-lang
	git(t, mate, "push", "-q")
	edit(t, ours, "# a\n\n**Trạng thái:** built\n\nour body\n", "ours") //znf:allow-lang
	head := git(t, ours, "rev-parse", "HEAD")

	notes := Sync(gitx.ExecRunner(), ours)
	if len(notes) != 1 || !strings.Contains(notes[0], "aborted, local commit kept intact") {
		t.Fatalf("notes = %v", notes)
	}
	if got := git(t, ours, "rev-parse", "HEAD"); got != head {
		t.Fatalf("local commit not kept: %s → %s", head, got)
	}
	if st := git(t, ours, "status", "--porcelain"); st != "" {
		t.Fatalf("store left mid-rebase: %q", st)
	}
}

// Update pulls a clean store that is behind its remote, so the sweep plans on the teammate's text.
func TestUpdate_PullsBehindStore(t *testing.T) {
	ours, mate := stores(t)
	edit(t, mate, "# a\n\n**Trạng thái:** superseded\n\nbody\n", "mate") //znf:allow-lang
	git(t, mate, "push", "-q")
	if _, err := Update(gitx.ExecRunner(), ours); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(ours, specPath)); !strings.Contains(string(b), "superseded") {
		t.Fatalf("store not brought up to date: %q", b)
	}
}

// A dirty store or a store with no upstream: dirty refuses, no upstream has nothing to pull.
func TestUpdate_DirtyRefusesNoUpstreamPasses(t *testing.T) {
	ours, _ := stores(t)
	writeFile(t, filepath.Join(ours, specPath), "dirty\n")
	if _, err := Update(gitx.ExecRunner(), ours); err == nil {
		t.Fatal("dirty store must refuse")
	}
	if _, err := Update(gitx.ExecRunner(), t.TempDir()); err != nil {
		t.Fatalf("non-repo must pass: %v", err)
	}
}

// A header change bundled with anything that is not a plain line edit under specs/ (here a new
// empty spec file, which shows no hunk line) is real content: the conflict aborts, nothing is dropped.
func TestSync_HeaderPlusStructuralChangeStillAborts(t *testing.T) {
	ours, mate := stores(t)
	edit(t, mate, "# a\n\n**Trạng thái:** superseded\n\nbody\n", "mate") //znf:allow-lang
	git(t, mate, "push", "-q")
	writeFile(t, filepath.Join(ours, specPath), "# a\n\n**Trạng thái:** built\n\nbody\n") //znf:allow-lang
	writeFile(t, filepath.Join(ours, "specs/r/2026-01-02-b-design.md"), "")
	git(t, ours, "add", "-A")
	git(t, ours, "commit", "-qm", "header + new empty spec")
	head := git(t, ours, "rev-parse", "HEAD")

	notes := Sync(gitx.ExecRunner(), ours)
	if len(notes) != 1 || !strings.Contains(notes[0], "aborted, local commit kept intact") {
		t.Fatalf("notes = %v", notes)
	}
	if got := git(t, ours, "rev-parse", "HEAD"); got != head {
		t.Fatalf("local commit not kept: %s → %s", head, got)
	}
}

// A status-like line in the spec body is real content, not the managed header: a conflict on it
// aborts and keeps our commit.
func TestSync_StatusLineInBodyStillAborts(t *testing.T) {
	ours, mate := stores(t)
	edit(t, mate, "# a\n\n**Trạng thái:** draft\n\nbody\n**Status:** mate example\n", "mate") //znf:allow-lang
	git(t, mate, "push", "-q")
	edit(t, ours, "# a\n\n**Trạng thái:** draft\n\nbody\n**Status:** our example\n", "ours") //znf:allow-lang
	head := git(t, ours, "rev-parse", "HEAD")

	notes := Sync(gitx.ExecRunner(), ours)
	if len(notes) != 1 || !strings.Contains(notes[0], "aborted, local commit kept intact") {
		t.Fatalf("notes = %v", notes)
	}
	if got := git(t, ours, "rev-parse", "HEAD"); got != head {
		t.Fatalf("local commit not kept: %s → %s", head, got)
	}
}

// A spec name with Vietnamese letters (git quotes it in patch output) is still recognised.
func TestSync_HeaderOnlyConflictNonASCIIPath(t *testing.T) {
	ours, mate := stores(t)
	p := "specs/r/2026-01-01-đăng-nhập-design.md"                          //znf:allow-lang
	writeFile(t, filepath.Join(ours, p), "# b\n\n**Trạng thái:** draft\n") //znf:allow-lang
	git(t, ours, "add", "-A")
	git(t, ours, "commit", "-qm", "b")
	git(t, ours, "push", "-q")
	git(t, mate, "pull", "-q")
	writeFile(t, filepath.Join(mate, p), "# b\n\n**Trạng thái:** superseded\n") //znf:allow-lang
	git(t, mate, "commit", "-qam", "mate")
	git(t, mate, "push", "-q")
	writeFile(t, filepath.Join(ours, p), "# b\n\n**Trạng thái:** built\n") //znf:allow-lang
	git(t, ours, "commit", "-qam", "sweep header")

	notes := Sync(gitx.ExecRunner(), ours)
	if len(notes) != 1 || !strings.HasPrefix(notes[0], "docs sync: pushed; dropped 1") {
		t.Fatalf("notes = %v", notes)
	}
}

// A spec with no header status whose only status-like line sits in the body: editing it is body
// content even when the value is one the sweep writes, so the conflict aborts.
func TestSync_BodyStatusWithoutHeaderStillAborts(t *testing.T) {
	ours, mate := stores(t)
	edit(t, ours, "# a\n\n## Notes\n**Status:** planned\n", "no header") //znf:allow-lang
	git(t, ours, "push", "-q")
	git(t, mate, "pull", "-q")
	edit(t, mate, "# a\n\n## Notes\n**Status:** superseded\n", "mate")
	git(t, mate, "push", "-q")
	edit(t, ours, "# a\n\n## Notes\n**Status:** built\n", "ours")
	head := git(t, ours, "rev-parse", "HEAD")

	notes := Sync(gitx.ExecRunner(), ours)
	if len(notes) != 1 || !strings.Contains(notes[0], "aborted, local commit kept intact") {
		t.Fatalf("notes = %v", notes)
	}
	if got := git(t, ours, "rev-parse", "HEAD"); got != head {
		t.Fatalf("local commit not kept: %s → %s", head, got)
	}
}
