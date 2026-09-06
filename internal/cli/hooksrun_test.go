package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	zdir := filepath.Join(ws, ".zenify")
	if err := os.MkdirAll(zdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zdir, "manifest.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return ws
}

// SC-6: outside a workspace => "{}" exit 0, no dispatch.
func TestHooksRun_OutsideWorkspace(t *testing.T) {
	outside := t.TempDir() // no .zenify
	if root, ok := findWorkspaceRoot(outside); ok {
		t.Fatalf("unexpectedly found workspace root %q", root)
	}
	var buf bytes.Buffer
	code := dispatchHook("docs-sync", "", &buf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(buf.String()) != "{}" {
		t.Fatalf("output = %q, want {}", buf.String())
	}
}

// SC-7: inside workspace => root found; dispatch reaches known id.
func TestHooksRun_InsideWorkspaceFindsRoot(t *testing.T) {
	ws := mkWorkspace(t)
	nested := filepath.Join(ws, "repos", "contact-center-be")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := findWorkspaceRoot(nested)
	if !ok {
		t.Fatal("expected to find workspace from nested dir")
	}
	if got != ws {
		t.Fatalf("root = %q, want %q", got, ws)
	}
}

// FR-5.2: unknown id => "{}" exit 0 (never crash user session).
func TestHooksRun_UnknownID(t *testing.T) {
	ws := mkWorkspace(t)
	var buf bytes.Buffer
	code := dispatchHook("totally-unknown", ws, &buf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(buf.String()) != "{}" {
		t.Fatalf("output = %q, want {}", buf.String())
	}
}
