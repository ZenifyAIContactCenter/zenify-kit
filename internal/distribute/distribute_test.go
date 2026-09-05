package distribute

import (
	"strings"
	"testing"
)

func TestParseManifest(t *testing.T) {
	in := []byte("# header\nCLAUDE.md  CLAUDE.md\n\nSYSTEM-MAP.md   .claude/SYSTEM-MAP.md\n  # indented comment\nbroken_line_no_dest\n")
	pairs, notes := ParseManifest(in)
	if len(pairs) != 2 {
		t.Fatalf("want 2 pairs, got %d: %+v", len(pairs), pairs)
	}
	if pairs[0] != (Pair{"CLAUDE.md", "CLAUDE.md"}) || pairs[1] != (Pair{"SYSTEM-MAP.md", ".claude/SYSTEM-MAP.md"}) {
		t.Fatalf("bad pairs: %+v", pairs)
	}
	if len(notes) != 1 {
		t.Fatalf("want 1 note for malformed line, got %v", notes)
	}
}

var errNotFound = errTest("not found")

type errTest string

func (e errTest) Error() string { return string(e) }

func TestPlanClassifies(t *testing.T) {
	src := map[string][]byte{
		"CLAUDE.md":     []byte("new content\n"),
		"SYSTEM-MAP.md": []byte("same\n"),
		"NEW.md":        []byte("brand new\n"), // nguồn có, đích vắng → CREATE
		// "MISSING.md" cố tình vắng → SKIP
	}
	dst := map[string][]byte{
		"CLAUDE.md":             []byte("old content\n"),
		".claude/SYSTEM-MAP.md": []byte("same\n"),
	}
	readSrc := func(p string) ([]byte, error) {
		if b, ok := src[p]; ok {
			return b, nil
		}
		return nil, errNotFound
	}
	readDst := func(p string) ([]byte, error) {
		if b, ok := dst[p]; ok {
			return b, nil
		}
		return nil, errNotFound
	}
	pairs := []Pair{
		{"CLAUDE.md", "CLAUDE.md"},
		{"SYSTEM-MAP.md", ".claude/SYSTEM-MAP.md"},
		{"NEW.md", "new/file.md"},
		{"MISSING.md", "whatever.md"},
	}
	got := Plan(pairs, readSrc, readDst, func(err error) bool { return err == errNotFound })
	want := []State{Update, Same, Create, Skip}
	for i, w := range want {
		if got[i].State != w {
			t.Errorf("pair %d: want %s got %s", i, w, got[i].State)
		}
	}
	if got[0].Diff == "" {
		t.Error("UPDATE must carry a non-empty diff")
	}
	if got[1].Diff != "" {
		t.Error("SAME must have empty diff")
	}
}

func TestPlanSkipsEscapingPaths(t *testing.T) {
	read := func(p string) ([]byte, error) { return []byte("x"), nil }
	pairs := []Pair{{"../evil", "ok.md"}, {"ok.md", "/etc/passwd"}, {"ok.md", "../../out.md"}}
	got := Plan(pairs, read, read, func(error) bool { return false })
	for i, p := range got {
		if p.State != Skip || p.Reason == "" {
			t.Errorf("cặp %d thoát-gốc phải SKIP kèm Reason, got %s %q", i, p.State, p.Reason)
		}
	}
}

func TestPlanDestUnreadableIsSkipNotCreate(t *testing.T) {
	read := func(p string) ([]byte, error) { return []byte("x"), nil }
	readDstErr := func(p string) ([]byte, error) { return nil, errTest("permission denied") }
	got := Plan([]Pair{{"a", "b"}}, read, readDstErr, func(err error) bool { return err == errNotFound })
	if got[0].State != Skip || got[0].Reason == "" {
		t.Fatalf("dest tồn-tại-không-đọc-được phải SKIP (không CREATE), got %s", got[0].State)
	}
}

func TestApplyWritesOnlyCreateUpdate(t *testing.T) {
	plans := []FilePlan{
		{Source: "a", Dest: "da", State: Create},
		{Source: "b", Dest: "db", State: Update},
		{Source: "c", Dest: "dc", State: Same},
		{Source: "d", Dest: "dd", State: Skip},
	}
	src := map[string][]byte{"a": []byte("A"), "b": []byte("B")}
	written := map[string][]byte{}
	readSrc := func(p string) ([]byte, error) { return src[p], nil }
	writeDst := func(dest string, data []byte) error { written[dest] = data; return nil }

	notes := Apply(plans, readSrc, writeDst)

	if len(written) != 2 || string(written["da"]) != "A" || string(written["db"]) != "B" {
		t.Fatalf("only CREATE+UPDATE should write: %v", written)
	}
	if _, ok := written["dc"]; ok {
		t.Error("SAME must not write")
	}
	if len(notes) != 2 {
		t.Errorf("want 2 write notes, got %v", notes)
	}
}

func TestApplyWriteFailureIsNote(t *testing.T) {
	plans := []FilePlan{{Source: "a", Dest: "da", State: Create}}
	readSrc := func(p string) ([]byte, error) { return []byte("A"), nil }
	writeDst := func(dest string, data []byte) error { return errTest("disk full") }
	notes := Apply(plans, readSrc, writeDst)
	if len(notes) != 1 || !strings.Contains(notes[0], "disk full") {
		t.Fatalf("write failure must be a note, got %v", notes)
	}
}
