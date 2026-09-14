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
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	writeWorktreeJSON(t, root, `{"abbrev":"myrepo","user":"namph","portRange":[3200,3249],"deps":"install","peers":`+peersJSON+`}`)
	wtPath = filepath.Join(root, ".worktrees", "my-task")
	if err := os.MkdirAll(wtPath, 0o750); err != nil {
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
	if err := os.MkdirAll(peerWt, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("FOO=bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")
	g.out[peerWt+"|config --get wt.port"] = "3311"
	g.out[wtPath+"|status --porcelain -- .env"] = ""

	if err := RunWire(wireOpts(root, wtPath, g, false)); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(wtPath, ".env")) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "API_URL=http://localhost:3311") {
		t.Fatalf("expected API_URL rewritten to peer port, got:\n%s", b)
	}
}

func TestRunWire_NoPeerWorktree_FallsBackToBaseline(t *testing.T) {
	root, wtPath := wireFixture(t, `{"API_URL":{"repo":"peer","url":"http://localhost:{port}"}}`)
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("API_URL=http://localhost:9999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("FOO=bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")
	g.out[wtPath+"|status --porcelain -- .env"] = ""

	if err := RunWire(wireOpts(root, wtPath, g, false)); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(wtPath, ".env")) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "API_URL=http://localhost:9999") {
		t.Fatalf("expected API_URL from baseline main env, got:\n%s", b)
	}
}

func TestRunWire_NoPeerNoBaseline_LeftAsIs(t *testing.T) {
	root, wtPath := wireFixture(t, `{"API_URL":{"repo":"peer","url":"http://localhost:{port}"}}`)
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("FOO=bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")

	if err := RunWire(wireOpts(root, wtPath, g, false)); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(wtPath, ".env")) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
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
	if err := os.MkdirAll(peerWt, 0o750); err != nil {
		t.Fatal(err)
	}
	original := "FOO=bar\nAPI_URL=http://localhost:3311"
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")
	g.out[peerWt+"|config --get wt.port"] = "3311"

	if err := RunWire(wireOpts(root, wtPath, g, false)); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(wtPath, ".env")) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
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
	if err := os.MkdirAll(peerWt, 0o750); err != nil {
		t.Fatal(err)
	}
	original := "FOO=bar\n"
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, wtPath, "my-task")
	g.out[peerWt+"|config --get wt.port"] = "3311"

	if err := RunWire(wireOpts(root, wtPath, g, true)); err != nil {
		t.Fatalf("RunWire: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(wtPath, ".env")) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != original {
		t.Fatalf("--dry-run must not write to disk, got:\n%s", b)
	}
}

func TestRunWire_NoPeersDeclared(t *testing.T) {
	root, wtPath := wireFixture(t, `{}`)
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("FOO=bar\n"), 0o600); err != nil {
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

// FR-7.2: a peer repo (web) declaring a var that points at THIS repo (hub) gets
// its same-slug worktree rewired when hub's worktree appears, and back to
// baseline when it disappears.
func TestRewirePeers_PointsPeerAtThisRepoAndBack(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "manifest.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	web := filepath.Join(ws, "repos", "web")
	hub := filepath.Join(ws, "repos", "hub")
	for _, d := range []string{web, hub} {
		if err := os.MkdirAll(filepath.Join(d, ".git"), 0o750); err != nil { // a .git DIR so gitstate.Scope finds it
			t.Fatal(err)
		}
	}
	writeWorktreeJSON(t, web, `{"abbrev":"web","user":"namph","portRange":[3300,3349],"deps":"none","peers":{"VITE_HUB_URL":{"repo":"hub","url":"http://localhost:{port}"}}}`)
	writeWorktreeJSON(t, hub, `{"abbrev":"hub","user":"namph","portRange":[3250,3299],"deps":"none"}`)
	webWt := filepath.Join(web, ".worktrees", "s1")
	hubWt := filepath.Join(hub, ".worktrees", "s1")
	for _, d := range []string{webWt, hubWt} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(web, ".env"), []byte("VITE_HUB_URL=http://localhost:3001\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webWt, ".env"), []byte("VITE_HUB_URL=http://localhost:3001\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	slugStub(g, webWt, "s1")
	g.out[hubWt+"|config --get wt.port"] = "3260"

	RewirePeers(ws, "hub", "s1", g, io.Discard, io.Discard)
	b, _ := os.ReadFile(filepath.Join(webWt, ".env")) //nolint:gosec // G304 -- test fixture path
	if !strings.Contains(string(b), "VITE_HUB_URL=http://localhost:3260") {
		t.Fatalf("web worktree must point at hub's worktree port, got %s", b)
	}

	// hub's worktree disappears (wt rm) → back to the main checkout baseline
	if err := os.RemoveAll(hubWt); err != nil {
		t.Fatal(err)
	}
	RewirePeers(ws, "hub", "s1", g, io.Discard, io.Discard)
	b, _ = os.ReadFile(filepath.Join(webWt, ".env")) //nolint:gosec // G304 -- test fixture path
	if !strings.Contains(string(b), "VITE_HUB_URL=http://localhost:3001") {
		t.Fatalf("after peer removal the var must return to baseline, got %s", b)
	}
}

func TestRunWire_DeterministicOrder(t *testing.T) {
	root, wtPath := wireFixture(t, `{"B_URL":{"repo":"peer","url":"http://localhost:{port}"},"A_URL":{"repo":"peer","url":"http://localhost:{port}"}}`)
	workspace := filepath.Dir(root)
	peerWt := filepath.Join(workspace, "peer", ".worktrees", "my-task")
	if err := os.MkdirAll(peerWt, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("FOO=bar\n"), 0o600); err != nil {
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
