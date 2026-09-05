package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// nopRunner: git không bao giờ được gọi khi workspace rỗng (không repo participant).
type nopRunner struct{}

func (nopRunner) Run(dir string, args ...string) ([]byte, error) { return nil, nil }

// SC-7: source production (không tính _test.go) của internal/release + internal/cli
// KHÔNG được hardcode định danh repo zenify → kit giữ project-agnostic.
func TestNoZenifyRepoIdentifiersInSource(t *testing.T) {
	// Cấm định danh repo THAM GIA release (danh sách này phải sống trong .znf/release-repos.txt).
	// Ngoại lệ có chủ đích: "zenify-knowledge" là repo record-layer (M6a) dùng làm out-dir mặc định,
	// và đã có cờ --out-dir để override cho workspace khác → không cấm.
	banned := []string{
		"contact-center-be", "contact-center-hub", "contact-center-web",
		"chatting", "notification", "personal-zalo-gateway", "change-stream-subscriber",
	}
	dirs := []string{".", "../release"}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			t.Fatalf("read dir %s: %v", d, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(d, name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			src := string(b)
			for _, id := range banned {
				if strings.Contains(src, id) {
					t.Errorf("%s/%s chứa định danh repo zenify %q — kit phải project-agnostic (đưa vào .znf/release-repos.txt)", d, name, id)
				}
			}
		}
	}
}

// FAIL-OPEN: workspace không có repo nào → vẫn ghi report rỗng, exit 0 (FR-5.2).
func TestRunReleaseReportFailOpenEmptyWorkspace(t *testing.T) {
	ws := t.TempDir()
	var out, errb bytes.Buffer
	if err := runReleaseReport(ws, 84, true, "", nopRunner{}, &out, &errb); err != nil {
		t.Fatalf("fail-open vi phạm: trả err %v", err)
	}
	path := filepath.Join(ws, "zenify-knowledge", "releases", "R84.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("report rỗng vẫn phải được ghi: %v", err)
	}
	if !strings.Contains(string(b), "# Release 84") {
		t.Errorf("report thiếu header: %s", b)
	}
}
