package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSpec writes a design file under <store>/specs/<repo>/.
func writeSpec(t *testing.T, store, repo, name, body string) {
	t.Helper()
	dir := filepath.Join(store, "specs", repo)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunSpecStatus_EmptyStoreExitsClean(t *testing.T) {
	store := t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "specs"), 0o750); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	err := runSpecStatus(store, "", false, true, "", true,
		func(string) (string, bool) { return "", false },
		func(string) string { return "origin/main" },
		&fakeGit{}, &out, &errb)
	if err != nil {
		t.Fatalf("err = %v, want nil (fail-open)", err)
	}
}

func TestRunSpecStatus_EmptyStorePrintsMessage(t *testing.T) {
	store := t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "specs"), 0o750); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	// jsonOut=false → table mode must print the empty-state message (SC-06).
	err := runSpecStatus(store, "", false, false, "", true,
		func(string) (string, bool) { return "", false },
		func(string) string { return "origin/main" },
		&fakeGit{}, &out, &errb)
	if err != nil {
		t.Fatalf("err = %v, want nil (fail-open)", err)
	}
	if !strings.Contains(out.String(), "không có spec") {
		t.Fatalf("empty store did not print the no-spec message (SC-06):\n%s", out.String())
	}
}

func TestRunSpecContracts_EmptyPrintsMessage(t *testing.T) {
	store := t.TempDir()
	writeSpec(t, store, "zenify-kit", "2026-09-09-b-design.md", "## Brief\n(no tags)\n")
	var out, errb bytes.Buffer
	err := runSpecContracts(store, "", "", false, &out, &errb)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), "không có contract") {
		t.Fatalf("empty registry did not print the no-contract message:\n%s", out.String())
	}
}

func TestRunSpecContracts_MissingTagCounted(t *testing.T) {
	store := t.TempDir()
	writeSpec(t, store, "zenify-kit", "2026-09-09-a-design.md",
		"## Brief\n_Blast-radius: zenify-kit only_\n_DB: N/A_\n")
	writeSpec(t, store, "zenify-kit", "2026-09-09-b-design.md", "## Brief\n(no tags)\n")
	var out, errb bytes.Buffer
	err := runSpecContracts(store, "", "", false, &out, &errb)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), "zenify-kit only") {
		t.Fatalf("out missing the tagged spec:\n%s", out.String())
	}
}
