package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGateParticipantsFiltersSharedStore(t *testing.T) {
	storeWithParticipants(t, "")
	ws := t.TempDir()
	// repo A: sharedStore true
	mkRepo(t, ws, "repoA", `{"abbrev":"a","gate":{"sharedStore":true,"dbAccessor":"db_read","accessPatterns":["models.mongo.*"]}}`)
	// repo B: sharedStore false
	mkRepo(t, ws, "repoB", `{"abbrev":"b","gate":{"sharedStore":false}}`)
	// junk directory that isn't a repo (no .claude/worktree.json) → skipped
	if err := os.MkdirAll(filepath.Join(ws, "notarepo"), 0o750); err != nil {
		t.Fatal(err)
	}
	ps, err := gateParticipants(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Name != "repoA" {
		t.Fatalf("participants = %+v, want only repoA", ps)
	}
	if ps[0].DBAccessor != "db_read" || len(ps[0].AccessPatterns) != 1 {
		t.Fatalf("repoA missing accessPatterns/dbAccessor: %+v", ps[0])
	}
}

func TestGateParticipantsFindsNested(t *testing.T) {
	storeWithParticipants(t, "")
	root := t.TempDir()
	repo := filepath.Join(root, "repos", "svc-a")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{"abbrev":"a","baseRef":"origin/staging","gate":{"sharedStore":true,"accessPatterns":["users"]}}`
	if err := os.WriteFile(filepath.Join(repo, ".claude", "worktree.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	ps, err := gateParticipants(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Name != "svc-a" {
		t.Fatalf("should find svc-a under repos/, got %+v", ps)
	}
}

func mkRepo(t *testing.T, ws, name, cfg string) {
	t.Helper()
	dir := filepath.Join(ws, name, ".claude")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worktree.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

func storeWithParticipants(t *testing.T, body string) {
	t.Helper()
	zh := t.TempDir()
	cfg := filepath.Join(zh, "knowledge", ".config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.WriteFile(filepath.Join(cfg, gateParticipantsFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ZENIFY_HOME", zh)
}

// SC-4: store file is the primary source; worktree.json repos are appended
// without duplicating a name already present.
func TestGateParticipants_StoreFileThenWorktreeMerge(t *testing.T) {
	storeWithParticipants(t, `[
	  {"name":"be","accessPatterns":["models.mongo."],"dbAccessor":"zenify db-read"},
	  {"name":"hub","accessPatterns":["@InjectModel("],"dbAccessor":"zenify db-read"}
	]`)
	ws := t.TempDir()
	mkRepo(t, ws, "hub", `{"abbrev":"h","gate":{"sharedStore":true,"dbAccessor":"other"}}`)
	mkRepo(t, ws, "nhb", `{"abbrev":"n","gate":{"sharedStore":true,"dbAccessor":"db_read"}}`)

	ps, err := gateParticipants(ws)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, p := range ps {
		names = append(names, p.Name)
	}
	if len(ps) != 3 || names[0] != "be" || names[1] != "hub" || names[2] != "nhb" {
		t.Fatalf("want [be hub nhb], got %v", names)
	}
	if ps[1].DBAccessor != "zenify db-read" {
		t.Fatalf("store entry must win over worktree.json for a duplicate name: %+v", ps[1])
	}
}

// FR-04.4: no store file and no participating repo → empty slice, which
// json-encodes as [] (never null).
func TestGateParticipants_EmptyIsSliceNotNil(t *testing.T) {
	storeWithParticipants(t, "")
	ps, err := gateParticipants(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ps == nil || len(ps) != 0 {
		t.Fatalf("want empty non-nil slice, got %#v", ps)
	}
	b, _ := json.Marshal(ps)
	if string(b) != "[]" {
		t.Fatalf("want [], got %s", b)
	}
}

// FR-04.2: a corrupt store file is reported on stderr and ignored.
func TestGateParticipants_CorruptStoreFileIgnored(t *testing.T) {
	storeWithParticipants(t, `{not json`)
	var e bytes.Buffer
	if got := storeParticipants(t.TempDir(), &e); len(got) != 0 {
		t.Fatalf("corrupt file must yield nothing, got %+v", got)
	}
	if !strings.Contains(e.String(), gateParticipantsFile) {
		t.Fatalf("expected a stderr note naming the file, got %q", e.String())
	}
}

// Ship-review finding: a read failure that is NOT "file missing" (here EISDIR —
// the participants file is a directory) must be reported, not treated as an
// empty list.
func TestGateParticipants_UnreadableStoreFileReported(t *testing.T) {
	storeWithParticipants(t, "")
	cfg := filepath.Join(os.Getenv("ZENIFY_HOME"), "knowledge", ".config")
	if err := os.Mkdir(filepath.Join(cfg, gateParticipantsFile), 0o755); err != nil {
		t.Fatal(err)
	}
	var e bytes.Buffer
	if got := storeParticipants(t.TempDir(), &e); len(got) != 0 {
		t.Fatalf("unreadable file must yield nothing, got %+v", got)
	}
	if !strings.Contains(e.String(), gateParticipantsFile) {
		t.Fatalf("expected a stderr note naming the file, got %q", e.String())
	}
}

// Ship-review finding: a store entry with no checkout under the workspace
// stays listed (the team list is the contract) — the missing-directory case is
// covered by the stderr note in gateParticipants, exercised here for the list
// shape only since that function writes to os.Stderr.
func TestGateParticipants_StoreEntryWithoutCheckoutStaysListed(t *testing.T) {
	storeWithParticipants(t, `[{"name":"ghost","accessPatterns":[".collection("],"dbAccessor":"zenify db-read"}]`)
	ps, err := gateParticipants(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Name != "ghost" {
		t.Fatalf("store entry must stay listed even without a checkout, got %+v", ps)
	}
}
