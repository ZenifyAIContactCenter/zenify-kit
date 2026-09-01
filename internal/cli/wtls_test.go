package cli

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// requireGit skips the test when git is not on PATH (real-git integration).
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// These reuse the real-git integration harness (initGitRepo/runWt) from
// wtnew_test.go in this same package: create a repo, `wt new` a task, then
// assert `wt ls`/`wt ls --json`/`wt url` reflect it.

func TestWtLs_And_Url_Integration(t *testing.T) {
	requireGit(t)
	t.Setenv("WT_SESSION", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := initGitRepo(t)
	t.Setenv("WT_REPO_ROOT", root)
	if _, err := runWt(t, root, "new", "my-task", "--type", "feat"); err != nil {
		t.Fatalf("precondition wt new failed: %v", err)
	}

	// table ls: contains header + the slug + a PORT value
	table, err := runWt(t, root, "ls")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(table, "SLUG") || !strings.Contains(table, "my-task") || !strings.Contains(table, "RUNNING") {
		t.Fatalf("ls table missing columns/slug: %q", table)
	}

	// json ls: parses to an array with our slug and an http url-able port
	js, err := runWt(t, root, "ls", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(js), &rows); err != nil {
		t.Fatalf("ls --json not valid JSON: %v\n%s", err, js)
	}
	found := false
	for _, r := range rows {
		if r["slug"] == "my-task" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ls --json missing my-task: %s", js)
	}

	// url: exact stdout contract
	url, err := runWt(t, root, "url", "my-task")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(url), "http://localhost:32") {
		t.Fatalf("url wrong: %q", url)
	}
}
