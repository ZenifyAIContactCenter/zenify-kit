package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

func TestSyncMaterializesTree(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	res, err := Sync(dest, man)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".claude-plugin", "plugin.json")); err != nil {
		t.Fatalf("plugin.json không được ghi: %v", err)
	}
	if len(res.Written) == 0 {
		t.Fatal("Written rỗng — không có file nào materialize")
	}
	res2, err := Sync(dest, man)
	if err != nil {
		t.Fatalf("Sync lần 2: %v", err)
	}
	if len(res2.Written) != 0 {
		t.Fatalf("sync idempotent phải Written=0, được %d", len(res2.Written))
	}
}

func TestSyncNeverEscapesDest(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	m, _ := managed.Load(man)
	for p := range m.Entries {
		if !strings.HasPrefix(p, dest) {
			t.Fatalf("ghi ra ngoài dest: %s", p)
		}
	}
}

func TestSyncKeepsUserAddedFile(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	// user tự tạo file trùng path một asset TRƯỚC khi sync, không qua manifest
	victim := filepath.Join(dest, ".claude-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(victim), 0o750); err != nil {
		t.Fatal(err)
	}
	userContent := []byte(`{"name":"user-edited"}`)
	if err := os.WriteFile(victim, userContent, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Sync(dest, man)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got, _ := os.ReadFile(victim); string(got) != string(userContent) {
		t.Fatalf("file người dùng bị ghi đè: %q", got)
	}
	var inKept bool
	for _, p := range res.Kept {
		if p == victim {
			inKept = true
		}
	}
	if !inKept {
		t.Fatalf("victim không nằm trong Kept: %v", res.Kept)
	}
}
