package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShipStep4_CitesVisualCheck(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills", "ship", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "zenify visual check") {
		t.Error("ship step-4 does not yet cite `zenify visual check`")
	}
	if !strings.Contains(string(b), "znf:review") {
		t.Error("ship SKILL lost znf:review after the edit")
	}
}
