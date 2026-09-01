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
