package uiverify

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func TestLoadConfig(t *testing.T) {
	cases := []struct {
		name     string
		file     *string
		declared bool
		want     Config
		badField string
	}{
		{name: "no file"},
		{name: "no uiVerify key", file: ptr(`{"baseRef":"origin/main"}`)},
		{name: "valid", file: ptr(`{"uiVerify":{"renderPaths":["src/"],"viewports":[1920,1366,1280]}}`),
			declared: true, want: Config{RenderPaths: []string{"src/"}, Viewports: []int{1920, 1366, 1280}}},
		{name: "empty renderPaths", file: ptr(`{"uiVerify":{"renderPaths":[]}}`), badField: "renderPaths"},
		{name: "blank entry", file: ptr(`{"uiVerify":{"renderPaths":[""]}}`), badField: "renderPaths"},
		{name: "absolute", file: ptr(`{"uiVerify":{"renderPaths":["/abs"]}}`), badField: "renderPaths"},
		{name: "dotdot", file: ptr(`{"uiVerify":{"renderPaths":["../x"]}}`), badField: "renderPaths"},
		{name: "inner dotdot", file: ptr(`{"uiVerify":{"renderPaths":["a/../b"]}}`), badField: "renderPaths"},
		{name: "dup viewport", file: ptr(`{"uiVerify":{"viewports":[1366,1366]}}`), badField: "viewports"},
		{name: "low viewport", file: ptr(`{"uiVerify":{"viewports":[200]}}`), badField: "viewports"},
		{name: "high viewport", file: ptr(`{"uiVerify":{"viewports":[4000]}}`), badField: "viewports"},
		{name: "broken json", file: ptr(`{"uiVerify":`), badField: "worktree.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			if tc.file != nil {
				p := filepath.Join(repo, ".claude", "worktree.json")
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(*tc.file), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			cfg, declared, err := LoadConfig(repo)
			if tc.badField != "" {
				if exitcode.Code(err) != exitcode.BadArgs {
					t.Fatalf("code = %d, err = %v; want BadArgs", exitcode.Code(err), err)
				}
				if !strings.Contains(err.Error(), tc.badField) {
					t.Fatalf("error %q lacks %q", err, tc.badField)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if declared != tc.declared || !reflect.DeepEqual(cfg, tc.want) {
				t.Fatalf("got (%+v, %v), want (%+v, %v)", cfg, declared, tc.want, tc.declared)
			}
		})
	}
}

func ptr(s string) *string { return &s }
