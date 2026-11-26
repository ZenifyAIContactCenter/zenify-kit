package infra

import (
	"context"
	"strings"
	"sync"
	"testing"
)

type recProgress struct {
	mu    sync.Mutex
	subs  []string
	lines []string
}

func (p *recProgress) Sub(s string) { p.mu.Lock(); p.subs = append(p.subs, s); p.mu.Unlock() }
func (p *recProgress) Output(l string) {
	p.mu.Lock()
	p.lines = append(p.lines, l)
	p.mu.Unlock()
}

// Business rule: the live build log a dev sees (and the run log on disk) must
// never carry a build secret, and must show the lines as they arrive.
func TestRunEnvStreamsRedactedLines(t *testing.T) {
	const token = "planted-build-value-0123456789"
	p := &recProgress{}
	ctx := WithProgress(context.Background(), p)
	script := `printf '#1 step one\n#2 auth %s\n#3 last' "$SECRET_X" >&2`
	if _, err := ExecBuildRunner().RunEnv(ctx, []string{"SECRET_X=" + token}, "sh", "-c", script); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(p.lines, "|")
	if got != "#1 step one|#2 auth [redacted]|#3 last" {
		t.Fatalf("lines = %q", got)
	}
}

// Business rule: long waits say what they wait for, so a stuck step is
// visible before its timeout.
func TestWaitsReportSubAction(t *testing.T) {
	p := &recProgress{}
	ctx := WithProgress(context.Background(), p)
	r := &fakeRunner{respond: hostFiles(desiredHostFiles(t), K3sVersion)}
	if err := waitBootstrapped(ctx, r); err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.subs, "|") != "waiting node Ready|waiting k3s CRDs Established" {
		t.Fatalf("subs = %v", p.subs)
	}
}
