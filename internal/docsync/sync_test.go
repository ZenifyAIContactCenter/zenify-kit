package docsync

import (
	"strings"
	"testing"
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
