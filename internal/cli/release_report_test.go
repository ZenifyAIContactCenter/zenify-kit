package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// nopRunner: git is never called when the workspace is empty (no participant repo).
type nopRunner struct{}

func (nopRunner) Run(dir string, args ...string) ([]byte, error) { return nil, nil }

// SC-7: production source (excluding _test.go) of internal/release + internal/cli
// must NOT hardcode a zenify repo identifier → keeps the kit project-agnostic.
func TestNoZenifyRepoIdentifiersInSource(t *testing.T) {
	// Ban identifiers of repos THAT PARTICIPATE in release (this list must live in .znf/release-repos.txt).
	// Deliberate exception: "docs" is the record-layer repo (M6a, renamed from zenify-knowledge)
	// used as the default out-dir, and already has the --out-dir flag to override → not banned.
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
					t.Errorf("%s/%s contains zenify repo identifier %q — kit must stay project-agnostic (move it into .znf/release-repos.txt)", d, name, id)
				}
			}
		}
	}
}

// FAIL-OPEN: workspace has no repo at all → still writes an empty report, exit 0 (FR-5.2).
// ZENIFY_HOME=t.TempDir() to isolate from the test machine's real ~/.zenify/knowledge.
func TestRunReleaseReportFailOpenEmptyWorkspace(t *testing.T) {
	ws := t.TempDir()
	zh := t.TempDir()
	t.Setenv("ZENIFY_HOME", zh)
	var out, errb bytes.Buffer
	if err := runReleaseReport(ws, 84, true, "", false, false, nopRunner{}, &out, &errb); err != nil {
		t.Fatalf("fail-open violated: returned err %v", err)
	}
	path := filepath.Join(zh, "knowledge", "releases", "R84.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("an empty report must still be written: %v", err)
	}
	if !strings.Contains(string(b), "# Release 84") {
		t.Errorf("report missing header: %s", b)
	}
}

// verbose=true on an empty workspace: still fail-open, report still written, no panic (FR-5.3).
func TestRunReleaseReportVerboseFailOpen(t *testing.T) {
	ws := t.TempDir()
	zh := t.TempDir()
	t.Setenv("ZENIFY_HOME", zh)
	var out, errb bytes.Buffer
	if err := runReleaseReport(ws, 84, true, "", true, false, nopRunner{}, &out, &errb); err != nil {
		t.Fatalf("fail-open violated (verbose): returned err %v", err)
	}
	path := filepath.Join(zh, "knowledge", "releases", "R84.md")
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("verbose report must still be written: %v", err)
	}
}

// --unreleased: writes unreleased.md (NOT R<n>.md) — FR-3/FR-4, SC-5.
func TestRunReleaseReportUnreleasedWritesUnreleasedFile(t *testing.T) {
	ws := t.TempDir()
	zh := t.TempDir()
	t.Setenv("ZENIFY_HOME", zh)
	var out, errb bytes.Buffer
	if err := runReleaseReport(ws, 88, true, "", false, true, nopRunner{}, &out, &errb); err != nil {
		t.Fatalf("fail-open violated: returned err %v", err)
	}
	unrelPath := filepath.Join(zh, "knowledge", "releases", "unreleased.md")
	if _, err := os.ReadFile(unrelPath); err != nil {
		t.Fatalf("must write unreleased.md: %v", err)
	}
	rPath := filepath.Join(zh, "knowledge", "releases", "R88.md")
	if _, err := os.ReadFile(rPath); err == nil {
		t.Errorf("--unreleased must NOT write R88.md")
	}
}

// FR-4.3: cutting a release (finalize, not --unreleased) also resets unreleased.md — fail-open.
func TestRunReleaseReportFinalizeResetsUnreleasedFile(t *testing.T) {
	ws := t.TempDir()
	zh := t.TempDir()
	t.Setenv("ZENIFY_HOME", zh)
	var out, errb bytes.Buffer
	if err := runReleaseReport(ws, 89, true, "", false, false, nopRunner{}, &out, &errb); err != nil {
		t.Fatalf("fail-open violated: returned err %v", err)
	}
	rPath := filepath.Join(zh, "knowledge", "releases", "R89.md")
	if _, err := os.ReadFile(rPath); err != nil {
		t.Fatalf("must write R89.md: %v", err)
	}
	unrelPath := filepath.Join(zh, "knowledge", "releases", "unreleased.md")
	if _, err := os.ReadFile(unrelPath); err != nil {
		t.Fatalf("finalize must reset unreleased.md: %v", err)
	}
}
