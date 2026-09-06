package docsync

import (
	"strings"
	"testing"
)

type fakeRunner struct {
	calls  [][]string
	status string // giá trị trả cho `status --porcelain`
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

// Sạch → pull, KHÔNG commit, KHÔNG push (idempotent, không empty commit).
func TestSync_CleanNoCommit(t *testing.T) {
	f := &fakeRunner{status: ""}
	Sync(f, "/docs")
	if ran(f.calls, "commit") {
		t.Fatalf("repo sạch không được commit; calls=%v", f.calls)
	}
	if !ran(f.calls, "pull --rebase") {
		t.Fatalf("phải pull --rebase; calls=%v", f.calls)
	}
}

// Dirty → add + commit + push.
func TestSync_DirtyCommitsAndPushes(t *testing.T) {
	f := &fakeRunner{status: " M specs/x.md"}
	Sync(f, "/docs")
	if !ran(f.calls, "add -A") || !ran(f.calls, "commit") || !ran(f.calls, "push") {
		t.Fatalf("dirty phải add+commit+push; calls=%v", f.calls)
	}
}

// Fail-open: pull lỗi vẫn tiếp tục, trả notes, không panic.
func TestSync_FailOpen(t *testing.T) {
	f := &fakeRunner{status: "", failOn: "pull"}
	notes := Sync(f, "/docs")
	if len(notes) == 0 {
		t.Fatal("pull lỗi phải sinh note")
	}
}
