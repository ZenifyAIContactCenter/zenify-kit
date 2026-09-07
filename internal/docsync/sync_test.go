package docsync

import (
	"strings"
	"testing"
)

type fakeRunner struct {
	calls  [][]string
	status string // giá trị trả cho `status --porcelain`
	ahead  string // giá trị trả cho `rev-list --count @{upstream}..HEAD` (rỗng = không xác định → 0)
	failOn string // arg[0] sẽ trả lỗi (rỗng = không lỗi)
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

// Sạch + ahead=0 → chỉ status + rev-list (đều LOCAL), KHÔNG network mutate:
// không pull, không commit, không push. Bảo chứng hook Stop chạy mỗi turn
// (sạch, up-to-date) không tốn round-trip mạng nào.
func TestSync_CleanIsLocalOnly(t *testing.T) {
	f := &fakeRunner{status: "", ahead: "0"}
	Sync(f, "/docs")
	if ran(f.calls, "commit") || ran(f.calls, "pull") || ran(f.calls, "push") {
		t.Fatalf("turn sạch+up-to-date không được pull/commit/push; calls=%v", f.calls)
	}
	if !ran(f.calls, "status --porcelain") {
		t.Fatalf("phải kiểm tra status; calls=%v", f.calls)
	}
}

// Sạch nhưng CÒN commit chưa push (clean-but-ahead) → KHÔNG commit thừa,
// nhưng phải pull --rebase + push để đẩy nốt commit kẹt từ lần trước.
func TestSync_CleanButAheadPushesPending(t *testing.T) {
	f := &fakeRunner{status: "", ahead: "2"}
	Sync(f, "/docs")
	if ran(f.calls, "add -A") || ran(f.calls, "commit") {
		t.Fatalf("sạch thì KHÔNG được add/commit thừa; calls=%v", f.calls)
	}
	if !ran(f.calls, "pull --rebase") || !ran(f.calls, "push") {
		t.Fatalf("clean-but-ahead phải pull--rebase+push commit kẹt; calls=%v", f.calls)
	}
}

// ahead không xác định (chưa set upstream → rev-list lỗi) → fail-safe về clean,
// KHÔNG đụng mạng (giữ nguyên hành vi cũ khi không biết ahead).
func TestSync_CleanAheadUnknownStaysLocal(t *testing.T) {
	f := &fakeRunner{status: "", failOn: "rev-list"}
	Sync(f, "/docs")
	if ran(f.calls, "pull") || ran(f.calls, "push") {
		t.Fatalf("không xác định ahead phải giữ local, không network; calls=%v", f.calls)
	}
}

// Dirty → commit TRƯỚC, rồi pull --rebase, rồi push (thứ tự commit-first).
func TestSync_DirtyCommitsThenRebasesThenPushes(t *testing.T) {
	f := &fakeRunner{status: " M specs/x.md"}
	Sync(f, "/docs")
	if !ran(f.calls, "add -A") || !ran(f.calls, "commit") ||
		!ran(f.calls, "pull --rebase") || !ran(f.calls, "push") {
		t.Fatalf("dirty phải add+commit+pull--rebase+push; calls=%v", f.calls)
	}
	// commit phải đứng TRƯỚC pull (commit-first, để rebase conflict abort được).
	var ci, pi int = -1, -1
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
		t.Fatalf("commit phải trước pull; commit@%d pull@%d calls=%v", ci, pi, f.calls)
	}
}

// Rebase conflict (pull lỗi) → rebase --abort, KHÔNG push (không publish marker).
func TestSync_RebaseConflictAbortsNoPush(t *testing.T) {
	f := &fakeRunner{status: " M specs/x.md", failOn: "pull"}
	Sync(f, "/docs")
	if !ran(f.calls, "rebase --abort") {
		t.Fatalf("pull lỗi phải rebase --abort; calls=%v", f.calls)
	}
	if ran(f.calls, "push") {
		t.Fatalf("pull lỗi KHÔNG được push (tránh publish rác); calls=%v", f.calls)
	}
}

// Fail-open: status lỗi vẫn trả notes, không panic, không network.
func TestSync_StatusFailOpen(t *testing.T) {
	f := &fakeRunner{failOn: "status"}
	notes := Sync(f, "/docs")
	if len(notes) == 0 {
		t.Fatal("status lỗi phải sinh note")
	}
	if ran(f.calls, "commit") || ran(f.calls, "push") {
		t.Fatalf("status lỗi không được commit/push; calls=%v", f.calls)
	}
}
