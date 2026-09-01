package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AKIAQWERTYUIOPASDFGH (not AKIAIOSFODNN7EXAMPLE from the task brief):
// gitleaks' default config allowlists any secret matching `.+EXAMPLE$"`
// (see internal/secretscan's own test comment), so the brief's literal
// EXAMPLE-suffixed value is never flagged — verified by running this test
// with that value first and observing zero findings.
const testSecret = "AKIAQWERTYUIOPASDFGH"

func TestSecretScanCmdFindsAndRedacts(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "leak.txt"), []byte("k = "+testSecret+"\n"), 0o644)
	cmd := newSecretScanCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{dir})
	err := cmd.Execute()
	if err == nil {
		t.Error("có secret phải trả lỗi (exit non-zero)")
	}
	combined := out.String() + errb.String()
	if !strings.Contains(combined, "leak.txt") {
		t.Errorf("output phải nêu file, được: %s", combined)
	}
	if strings.Contains(combined, testSecret) {
		t.Error("output lộ secret nguyên (vi phạm FR-041)")
	}
}

func TestSecretScanCmdClean(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("hello world\n"), 0o644)
	cmd := newSecretScanCmd()
	cmd.SetArgs([]string{dir})
	if err := cmd.Execute(); err != nil {
		t.Errorf("cây sạch phải exit 0, được %v", err)
	}
}
