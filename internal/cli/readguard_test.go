package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
)

func noRecord(string, string, int64, time.Time) observe.Advice { return observe.Advice{} }

func TestRunReadGuard_DeniesBigTextAndRecords(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.md")
	if err := os.WriteFile(p, make([]byte, 250_000), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotTool string
	var gotBytes int64
	rec := func(_ string, tool string, b int64, _ time.Time) observe.Advice {
		gotTool, gotBytes = tool, b
		return observe.Advice{}
	}
	in := strings.NewReader(`{"session_id":"s1","tool_name":"Read","tool_input":{"file_path":"` + p + `"}}`)
	var errb bytes.Buffer
	if code := runReadGuard(in, &errb, os.Stat, rec); code != 2 {
		t.Fatalf("code=%d want 2; stderr=%q", code, errb.String())
	}
	if !strings.Contains(errb.String(), "offset") {
		t.Fatalf("stderr must hint offset: %q", errb.String())
	}
	if gotTool != "Read:denied" || gotBytes != 250_000 {
		t.Fatalf("record got %q %d", gotTool, gotBytes)
	}
}

func TestRunReadGuard_AllowsWithRange(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.md")
	if err := os.WriteFile(p, make([]byte, 250_000), 0o600); err != nil {
		t.Fatal(err)
	}
	in := strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":"` + p + `","offset":10,"limit":50}}`)
	var errb bytes.Buffer
	if code := runReadGuard(in, &errb, os.Stat, noRecord); code != 0 || errb.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
}

func TestRunReadGuard_FailsOpen(t *testing.T) {
	var errb bytes.Buffer
	if code := runReadGuard(strings.NewReader("{not json"), &errb, os.Stat, noRecord); code != 0 || errb.Len() != 0 {
		t.Fatalf("bad json: code=%d stderr=%q", code, errb.String())
	}
	if code := runReadGuard(strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":"/nope/none.md"}}`), &errb, os.Stat, noRecord); code != 0 {
		t.Fatalf("missing file: code=%d", code)
	}
	boom := func(string) (os.FileInfo, error) { panic("stat exploded") }
	if code := runReadGuard(strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":"/x"}}`), &errb, boom, noRecord); code != 0 {
		t.Fatalf("panic must fall open: code=%d", code)
	}
}

func TestDispatchHook_ReadGuardIsRouted(t *testing.T) {
	// dispatchHook with an unknown id prints "{}"; read-guard must NOT be unknown.
	var out bytes.Buffer
	dispatchHook("read-guard", t.TempDir(), &out)
	if strings.TrimSpace(out.String()) == "{}" {
		t.Fatal("read-guard fell into the unknown-id noop")
	}
}
