package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SC-7: non-test Go source của internal/distribute + cli/config.go không hardcode định danh repo zenify.
func TestConfigNoZenifyRepoIdentifiers(t *testing.T) {
	banned := []string{
		"contact-center-be", "contact-center-hub", "contact-center-web",
		"chatting", "notification", "personal-zalo-gateway", "change-stream-subscriber",
	}
	files := []string{"config.go", "../distribute/distribute.go"}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, id := range banned {
			if strings.Contains(string(b), id) {
				t.Errorf("%s chứa định danh repo zenify %q — path phải nằm trong manifest", f, id)
			}
		}
	}
}

// FAIL-OPEN: config dir không tồn tại → exit 0, không panic.
func TestRunConfigFailOpenNoConfigDir(t *testing.T) {
	ws := t.TempDir()
	var out, errb bytes.Buffer
	if err := runConfig(ws, filepath.Join(ws, "nope"), false, &out, &errb); err != nil {
		t.Fatalf("fail-open vi phạm: %v", err)
	}
}

// SC-1 + SC-2 + SC-3: dry-run không ghi; apply tạo CREATE; chạy lại = SAME (idempotent).
func TestRunConfigDryRunThenApplyIdempotent(t *testing.T) {
	ws := t.TempDir()
	cfg := filepath.Join(ws, "cfgdir")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(cfg, "CLAUDE.md"), []byte("hello\n"), 0o644)
	os.WriteFile(filepath.Join(cfg, "distribution.txt"), []byte("CLAUDE.md CLAUDE.md\n"), 0o644)
	dest := filepath.Join(ws, "CLAUDE.md")

	var o1, e1 bytes.Buffer
	runConfig(ws, cfg, false, &o1, &e1)
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("dry-run phải không ghi (SC-1)")
	}
	var o2, e2 bytes.Buffer
	runConfig(ws, cfg, true, &o2, &e2)
	b, err := os.ReadFile(dest)
	if err != nil || string(b) != "hello\n" {
		t.Fatalf("apply phải tạo dest với đúng nội dung: %v %q", err, b)
	}
	var o3, e3 bytes.Buffer
	runConfig(ws, cfg, false, &o3, &e3)
	if !strings.Contains(o3.String(), "SAME") {
		t.Fatalf("chạy lại phải là SAME (SC-2): %s", o3.String())
	}
}
