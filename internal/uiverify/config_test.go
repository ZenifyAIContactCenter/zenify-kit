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
		{name: "dot slash", file: ptr(`{"uiVerify":{"renderPaths":["./src/"]}}`), badField: "renderPaths"},
		{name: "double slash", file: ptr(`{"uiVerify":{"renderPaths":["src//"]}}`), badField: "renderPaths"},
		{name: "inner double slash", file: ptr(`{"uiVerify":{"renderPaths":["src//views"]}}`), badField: "renderPaths"},
		{name: "glob star", file: ptr(`{"uiVerify":{"renderPaths":["src/**"]}}`), badField: "renderPaths"},
		{name: "glob ext", file: ptr(`{"uiVerify":{"renderPaths":["src/*.tsx"]}}`), badField: "renderPaths"},
		{name: "glob question", file: ptr(`{"uiVerify":{"renderPaths":["src?"]}}`), badField: "renderPaths"},
		{name: "glob class", file: ptr(`{"uiVerify":{"renderPaths":["src/[ab]"]}}`), badField: "renderPaths"},
		{name: "backslash", file: ptr(`{"uiVerify":{"renderPaths":["src\\views"]}}`), badField: "renderPaths"},
		{name: "dot", file: ptr(`{"uiVerify":{"renderPaths":["."]}}`), badField: "renderPaths"},
		{name: "dot slash only", file: ptr(`{"uiVerify":{"renderPaths":["./"]}}`), badField: "renderPaths"},
		{name: "inner dot", file: ptr(`{"uiVerify":{"renderPaths":["src/./views"]}}`), badField: "renderPaths"},
		{name: "slash only", file: ptr(`{"uiVerify":{"renderPaths":["/"]}}`), badField: "renderPaths"},
		{name: "plain prefixes accepted", file: ptr(`{"uiVerify":{"renderPaths":["src/","src","src/views/"]}}`),
			declared: true, want: Config{RenderPaths: []string{"src/", "src", "src/views/"}}},
		{name: "unknown uiVerify key", file: ptr(`{"uiVerify":{"viewport":[1920]}}`), badField: `"viewport"`},
		{name: "unknown renderPath key", file: ptr(`{"uiVerify":{"renderPath":["src/"]}}`), badField: `"renderPath"`},
		{name: "unknown keys outside uiVerify allowed", file: ptr(`{"baseRef":"origin/main","setup":["npm ci"],"uiVerify":{"viewports":[1280]}}`),
			declared: true, want: Config{Viewports: []int{1280}}},
		{name: "dup viewport", file: ptr(`{"uiVerify":{"viewports":[1366,1366]}}`), badField: "viewports"},
		{name: "low viewport", file: ptr(`{"uiVerify":{"viewports":[200]}}`), badField: "viewports"},
		{name: "high viewport", file: ptr(`{"uiVerify":{"viewports":[4000]}}`), badField: "viewports"},
		{name: "style valid", file: ptr(`{"uiVerify":{"style":["hex","palette"]}}`),
			declared: true, want: Config{Style: []string{StyleHex, StylePalette}}},
		{name: "style unknown", file: ptr(`{"uiVerify":{"style":["hexx"]}}`), badField: "uiVerify.style"},
		{name: "style empty", file: ptr(`{"uiVerify":{"style":[]}}`), badField: "uiVerify.style"},
		{name: "style duplicate", file: ptr(`{"uiVerify":{"style":["hex","hex"]}}`), badField: "uiVerify.style"},
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
