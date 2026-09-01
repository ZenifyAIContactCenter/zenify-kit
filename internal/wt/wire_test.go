package wt

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wireFixture builds workspace/<repo> (the main checkout, with the peers
// config) and workspace/peer (a sibling repo). Returns the repo root and the
// worktree path (workspace/<repo>/.worktrees/<slug>).
func wireFixture(t *testing.T, peersJSON string) (root, wtPath string) {
	t.Helper()
	workspace := t.TempDir()
	root = filepath.Join(workspace, "myrepo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorktreeJSON(t, root, `{"abbrev":"myrepo","user":"namph","portRange":[3200,3249],"deps":"install","peers":`+peersJSON+`}`)
	wtPath = filepath.Join(root, ".worktrees", "my-task")
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, wtPath
}

func wireOpts(root, wtPath string, g *gitStub, dryRun bool) WireOptions {
	return WireOptions{
		RepoRoot: root, WorktreePath: wtPath, DryRun: dryRun,
		Runner: g, Stdout: io.Discard, Stderr: io.Discard,
	}
}

func slugStub(g *gitStub, wtPath, slug string) {
	g.out[wtPath+"|config --get wt.slug"] = slug
}

func TestRunWire_PeerWorktreeExists(t *testing.T) {
	root, wtPath := wireFixture(t, `{"API_URL":{"repo":"peer","url":"http://localhost:{port}"}}`)
	workspace := filepath.Dir(root)
	peerWt := filepath.Join(workspace, "peer", ".worktrees", "my-task")
	if err := os.MkdirAll(peerWt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")
	g.out[peerWt+"|config --get wt.port"] = "3311"
	g.out[wtPath+"|status --porcelain -- .env"] = ""

	if err := RunWire(wireOpts(root, wtPath, g, false)); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(wtPath, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "API_URL=http://localhost:3311") {
		t.Fatalf("expected API_URL rewritten to peer port, got:\n%s", b)
	}
}

func TestRunWire_NoPeerWorktree_FallsBackToBaseline(t *testing.T) {
	root, wtPath := wireFixture(t, `{"API_URL":{"repo":"peer","url":"http://localhost:{port}"}}`)
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("API_URL=http://localhost:9999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")
	g.out[wtPath+"|status --porcelain -- .env"] = ""

	if err := RunWire(wireOpts(root, wtPath, g, false)); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(wtPath, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "API_URL=http://localhost:9999") {
		t.Fatalf("expected API_URL from baseline main env, got:\n%s", b)
	}
}

func TestRunWire_NoPeerNoBaseline_LeftAsIs(t *testing.T) {
	root, wtPath := wireFixture(t, `{"API_URL":{"repo":"peer","url":"http://localhost:{port}"}}`)
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")

	if err := RunWire(wireOpts(root, wtPath, g, false)); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(wtPath, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "API_URL") {
		t.Fatalf("API_URL must be left unset with no peer and no baseline, got:\n%s", b)
	}
}

func TestRunWire_AlreadyCorrect_NoChange(t *testing.T) {
	root, wtPath := wireFixture(t, `{"API_URL":{"repo":"peer","url":"http://localhost:{port}"}}`)
	workspace := filepath.Dir(root)
	peerWt := filepath.Join(workspace, "peer", ".worktrees", "my-task")
	if err := os.MkdirAll(peerWt, 0o755); err != nil {
		t.Fatal(err)
	}
	original := "FOO=bar\nAPI_URL=http://localhost:3311"
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")
	g.out[peerWt+"|config --get wt.port"] = "3311"

	if err := RunWire(wireOpts(root, wtPath, g, false)); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(wtPath, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != original {
		t.Fatalf("file must stay byte-identical when already correct, got:\n%s", b)
	}
}

func TestRunWire_DryRun_LeavesDiskUntouched(t *testing.T) {
	root, wtPath := wireFixture(t, `{"API_URL":{"repo":"peer","url":"http://localhost:{port}"}}`)
	workspace := filepath.Dir(root)
	peerWt := filepath.Join(workspace, "peer", ".worktrees", "my-task")
	if err := os.MkdirAll(peerWt, 0o755); err != nil {
		t.Fatal(err)
	}
	original := "FOO=bar\n"
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")
	g.out[peerWt+"|config --get wt.port"] = "3311"

	if err := RunWire(wireOpts(root, wtPath, g, true)); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(wtPath, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != original {
		t.Fatalf("--dry-run must not write to disk, got:\n%s", b)
	}
}

func TestRunWire_NoPeersDeclared(t *testing.T) {
	root, wtPath := wireFixture(t, `{}`)
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")

	if err := RunWire(wireOpts(root, wtPath, g, false)); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
}

func TestRunWire_NotInsideWorktree_Errors(t *testing.T) {
	root, wtPath := wireFixture(t, `{"API_URL":{"repo":"peer","url":"http://localhost:{port}"}}`)
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	// wt.slug lookup errors → cfgGet returns "" → RunWire must refuse.
	g.err[wtPath+"|config --get wt.slug"] = errors.New("no slug configured")

	if err := RunWire(wireOpts(root, wtPath, g, false)); err == nil {
		t.Fatal("expected error when not inside a wt worktree")
	}
}

func TestRunWire_DeterministicOrder(t *testing.T) {
	root, wtPath := wireFixture(t, `{"B_URL":{"repo":"peer","url":"http://localhost:{port}"},"A_URL":{"repo":"peer","url":"http://localhost:{port}"}}`)
	workspace := filepath.Dir(root)
	peerWt := filepath.Join(workspace, "peer", ".worktrees", "my-task")
	if err := os.MkdirAll(peerWt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")
	g.out[peerWt+"|config --get wt.port"] = "3311"
	g.out[wtPath+"|status --porcelain -- .env"] = ""

	var out strings.Builder
	o := wireOpts(root, wtPath, g, false)
	o.Stdout = &out
	if err := RunWire(o); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
	text := out.String()
	ia := strings.Index(text, "A_URL=")
	ib := strings.Index(text, "B_URL=")
	if ia < 0 || ib < 0 || ia > ib {
		t.Fatalf("expected A_URL before B_URL in output, got:\n%s", text)
	}
}
