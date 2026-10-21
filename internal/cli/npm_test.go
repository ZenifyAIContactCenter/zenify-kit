package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func TestNpmLogin_NoRepoNeedStillSetsUp(t *testing.T) {
	zshrc, _ := npmTokenTestEnv(t, false)
	var out bytes.Buffer
	err := npmLogin(&out, stubGH{auth: []byte(npmTokenTestAuthWithScope)}, currentShellProfile, false, func() error { return nil })
	if err != nil {
		t.Fatalf("npmLogin: %v", err)
	}
	if !strings.Contains(npmTokenTestRead(t, zshrc), npmTokenLine) {
		t.Fatalf("profile lacks the npm token line: %q", npmTokenTestRead(t, zshrc))
	}
	if !strings.Contains(out.String(), "npm token: ok") {
		t.Fatalf("output lacks ok: %q", out.String())
	}
	if strings.Contains(out.String(), "ghp_FAKE") {
		t.Fatalf("output leaked the token: %q", out.String())
	}
}

func TestNpmLogin_NoTTYMissingScope(t *testing.T) {
	npmTokenTestEnv(t, false)
	refreshed := 0
	var out bytes.Buffer
	err := npmLogin(&out, stubGH{auth: []byte(npmTokenTestAuthNoScope)}, currentShellProfile, false, func() error { refreshed++; return nil })
	if err == nil {
		t.Fatal("expected an error")
	}
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("exit code = %d, want Fail", exitcode.Code(err))
	}
	if !strings.Contains(err.Error(), "read:packages") {
		t.Fatalf("error lacks read:packages: %v", err)
	}
	if refreshed != 0 {
		t.Fatalf("refresh ran %d times without a TTY", refreshed)
	}
}

func TestNpmTokenFix_RepairsAndKeepsStdoutClean(t *testing.T) {
	zshrc, _ := npmTokenTestEnv(t, true)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })
	refreshed := 0
	fix := npmTokenFix(stubGH{auth: []byte(npmTokenTestAuthWithScope)}, currentShellProfile, func() bool { return false }, func() error { refreshed++; return nil })
	ok, detail := fix()
	os.Stdout = old
	_ = w.Close()
	got, _ := io.ReadAll(r)
	if !ok {
		t.Fatalf("fix failed: %s", detail)
	}
	if !strings.Contains(npmTokenTestRead(t, zshrc), npmTokenLine) {
		t.Fatalf("profile lacks the npm token line")
	}
	if len(got) != 0 {
		t.Fatalf("fix wrote to stdout: %q", got)
	}
}

func TestGhRefreshPackages_Wiring(t *testing.T) {
	var out bytes.Buffer
	cmd := ghRefreshPackagesCmd(&out)
	if got := strings.Join(cmd.Args, " "); got != "gh auth refresh -h github.com -s read:packages" {
		t.Fatalf("args = %q", got)
	}
	if cmd.Stdout != &out {
		t.Fatal("Stdout is not the given writer")
	}
	if cmd.Stdin != os.Stdin || cmd.Stderr != os.Stderr {
		t.Fatal("Stdin/Stderr not wired to the process")
	}
	if ghRefreshPackages(&out) == nil {
		t.Fatal("nil closure")
	}
}
